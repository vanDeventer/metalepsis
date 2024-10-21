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
	"log"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
	"github.com/sdoque/mbaigo/usecases"
)

// Define the types of requests the serviceRegistry manager can handle
type ServiceRegistryRequest struct {
	Action string
	Record forms.ServiceRecord_v1
	Id     int
	Result chan []forms.ServiceRecord_v1 // For returning records (read operations)
	Error  chan error                    // For error handling
}

//-------------------------------------Define the unit asset

// UnitAsset type models the unit asset (interface) of the system
type UnitAsset struct {
	Name        string              `json:"name"`
	Owner       *components.System  `json:"-"`
	Details     map[string][]string `json:"details"`
	ServicesMap components.Services `json:"-"`
	CervicesMap components.Cervices `json:"-"`
	//
	serviceRegistry  map[int]forms.ServiceRecord_v1
	uRecCount        int
	requests         chan ServiceRegistryRequest
	sched            *Scheduler
	leading          bool
	leadingSince     time.Time
	leadingRegistrar *components.CoreSystem // if not leading this is the current leader
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
		Details:     map[string][]string{"Forms": usecases.ServQustForms()},
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
	// Start the registration expiration check scheduler
	cleaningScheduler := NewScheduler()
	go cleaningScheduler.run()

	// Initialize the UnitAsset
	ua := &UnitAsset{
		Name:            uac.Name,
		Owner:           sys,
		Details:         uac.Details,
		serviceRegistry: make(map[int]forms.ServiceRecord_v1),
		sched:           cleaningScheduler,
		ServicesMap:     components.CloneServices(servs),
		requests:        make(chan ServiceRegistryRequest), // Initialize the requests channel
	}

	ua.Role() // Start to repeatedly check which is the leading registrar

	// Start the service registry manager goroutine
	go serviceRegistryManager(ua.serviceRegistry, ua.requests)

	return ua, func() {
		// Close channels before exiting (cleanup)
		close(ua.requests)
		log.Println("Closing the service registry database connection")
	}
}

//-------------------------------------Unit's resource methods

// There are really two assets here: the database  and the scheduler
// The scheduler is (protected) in a third file: scheduler.go

// ServiceRegistryManager manages all service registry operations via channels
func serviceRegistryManager(serviceRegistry map[int]forms.ServiceRecord_v1, requests chan ServiceRegistryRequest) {
	for request := range requests {
		switch request.Action {
		case "add":
			// Handle add record
			serviceRegistry[request.Record.Id] = request.Record
			request.Error <- nil // Send success response

		case "read":
			// Handle read records
			var result []forms.ServiceRecord_v1
			for _, record := range serviceRegistry {
				result = append(result, record)
			}
			request.Result <- result

		case "delete":
			// Handle delete record
			delete(serviceRegistry, request.Id)
			request.Error <- nil // Send success response
		}
	}
}
