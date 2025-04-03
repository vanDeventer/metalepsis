/*******************************************************************************
 * Copyright (c) 2024 Jan van Deventer
 *
 * All rights reserved. This program and the accompanying materials
 * are made available under the terms of the Eclipse Public License v2.0
 * which accompanies this distribution, and is available at
 * http://www.eclipse.org/legal/epl-2.0/
 *
 * Contributors:
 *   Jan A. van Deventer, Luleå - initial implementation
 *   Thomas Hedeler, Hamburg - initial implementation
 ***************************************************************************SDG*/

package main

import (
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/usecases"
)

//-------------------------------------Define the unit asset

// UnitAsset type models the unit asset (interface) of the system
type UnitAsset struct {
	Name        string              `json:"name"`
	Owner       *components.System  `json:"-"`
	Details     map[string][]string `json:"details"`
	ServicesMap components.Services `json:"-"`
	CervicesMap components.Cervices `json:"-"`
	//
	db               *sql.DB                `json:"-"`
	sched            *Scheduler             `json:"-"`
	mtx              *sync.RWMutex          `json:"-"`
	leading          bool                   `json:"-"`
	leadingSince     time.Time              `json:"-"`
	leadingRegistrar *components.CoreSystem `json:"-"` // if not leading this is the current leader
}

// GetName returns the name of the Resource.
func (ua *UnitAsset) GetName() string {
	return ua.Name
}

// GetServices returns the services of the Resource.
func (ua *UnitAsset) GetServices() components.Services {
	return ua.ServicesMap
}

// GetCervices returns the list of consumed services by the Resource.
func (ua *UnitAsset) GetCervices() components.Cervices {
	return ua.CervicesMap
}

// GetDetails returns the details of the Resource.
func (ua *UnitAsset) GetDetails() map[string][]string {
	return ua.Details
}

// ensure UnitAsset implements components.UnitAsset (this check is done at during the compilation)
var _ components.UnitAsset = (*UnitAsset)(nil)

//-------------------------------------Instatiate a unit asset template

// initTemplate initializes a UnitAsset with default values.
func initTemplate() components.UnitAsset {
	// Define the services that expose the capabilities of the unit asset(s)
	registerService := components.Service{
		Definition:  "register",
		SubPath:     "register",
		Details:     map[string][]string{"Forms": usecases.ServiceRegistrationFormsList()},
		Description: "registers a service (POST) or updates its expiration time (PUT)",
	}

	queryService := components.Service{
		Definition:  "query",
		SubPath:     "query",
		Details:     map[string][]string{"Forms": usecases.ServQuestForms()},
		Description: "retrieves all currently available services using a GET request [accessed via a browser by a deployment technician] or retrieves a specific set of services using a POST request with a payload [initiated by the Orchestrator]",
	}
	unregisterService := components.Service{
		Definition:  "unregister",
		SubPath:     "unregister",
		Details:     map[string][]string{"Forms": {"ID only"}},
		Description: "removes a record (DELETE) based on record ID",
	}

	statusService := components.Service{
		Definition:  "status",
		SubPath:     "status",
		Details:     map[string][]string{"Forms": {"none"}},
		Description: "reports (GET) the role of the Service Registrar as leading or on stand by",
	}

	// var uat components.UnitAsset // this is an interface, which we then initialize
	uat := &UnitAsset{
		Name:    "registry",
		Details: map[string][]string{"Location": {"Local cloud"}},
		ServicesMap: components.Services{
			registerService.SubPath:   &registerService,
			queryService.SubPath:      &queryService,
			unregisterService.SubPath: &unregisterService,
			statusService.SubPath:     &statusService,
		},
	}
	return uat
}

//-------------------------------------Instatiate unit asset(s) based on configuration

// newResource creates the unit asset with its pointers and channels based on the configuration using the uaConfig structs
func newResource(uac UnitAsset, sys *components.System, servs []components.Service) (components.UnitAsset, func()) {

	//create a new service registry database
	serviceDB, err := createDB()
	if err != nil {
		panic(err)
	}

	// instantiate a read write mutex to ensure that only one record is open at the time
	var rwmtx sync.RWMutex

	// Start the registration expiration check scheduler
	cleaningScheduler := NewScheduler()
	go cleaningScheduler.run()

	// var ua components.UnitAsset // this is an interface, which we then initialize
	ua := &UnitAsset{ // this is an interface, which we then initialize
		Name:        uac.Name,
		Owner:       sys,
		Details:     uac.Details,
		db:          serviceDB,
		mtx:         &rwmtx,
		sched:       cleaningScheduler,
		ServicesMap: components.CloneServices(servs),
	}

	ua.Role() // start to repeatedly look which is the leading registrar

	return ua, func() {
		ua.db.Close()
		log.Println("Closing the service registry database connection")
	}
}

//-------------------------------------Unit's resource methods

// There are two parts here: the database functions and the scheduler functions
// To simplify things, they are in their own files (as an exception to the two files usually found with the AiGo systems)
