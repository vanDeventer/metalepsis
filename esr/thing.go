/*******************************************************************************
 * Copyright (c) 2024 Synecdoque
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, subject to the following conditions:
 *
 * The software is licensed under the MIT License. See the LICENSE file in this repository for details.
 *
 * Contributors:
 *   Jan A. van Deventer, Luleå - initial implementation
 *   Thomas Hedeler, Hamburg - initial implementation
 ***************************************************************************SDG*/

package main

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
	"github.com/sdoque/mbaigo/usecases"
)

// Define the types of requests the serviceRegistry manager can handle
type ServiceRegistryRequest struct {
	Action string
	Record forms.Form
	Id     int64
	Result chan []forms.ServiceRecord_v1 // For returning records
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
	mu               sync.Mutex
	recCount         int64
	requests         chan ServiceRegistryRequest
	sched            *Scheduler
	leading          bool
	leadingSince     time.Time
	leadingRegistrar *components.CoreSystem // if not leading this points to the current leader
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

//-------------------------------------Instantiate a unit asset template

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
		Details:     map[string][]string{"Forms": {"ID_only"}},
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
		Details: map[string][]string{"Location": {"LocalCloud"}},
		ServicesMap: components.Services{
			registerService.SubPath:   &registerService,
			queryService.SubPath:      &queryService,
			unregisterService.SubPath: &unregisterService,
			statusService.SubPath:     &statusService,
		},
	}
	return uat
}

//-------------------------------------Instantiate unit asset(s) based on configuration

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
		recCount:        1, // 0 is used for non registered services
		sched:           cleaningScheduler,
		ServicesMap:     components.CloneServices(servs),
		requests:        make(chan ServiceRegistryRequest), // Initialize the requests channel
	}

	ua.Role() // Start to repeatedly check which is the leading registrar

	// Start the service registry manager goroutine
	go ua.serviceRegistryHandler()

	return ua, func() {
		close(ua.requests)       // Close channels before exiting (cleanup)
		cleaningScheduler.Stop() // Gracefully stop the scheduler
		log.Println("Closing the service registry database connection")
	}
}

//-------------------------------------Unit's resource methods

// There are really two assets here: the database  and the scheduler
// The scheduler is (protected) in a third file: scheduler.go

// ServiceRegistryManager manages all service registry operations via channels
func (ua *UnitAsset) serviceRegistryHandler() {
	for request := range ua.requests {
		now := time.Now()
		switch request.Action {
		case "add":
			rec, ok := request.Record.(*forms.ServiceRecord_v1)
			if !ok {
				fmt.Println("Problem unpacking the service registration request")
				request.Error <- fmt.Errorf("invalid record type")
				continue
			}
			ua.mu.Lock() // Lock the serviceRegistry map

			if rec.Id == 0 {
				// In the case recCount had looped, check that there is no record at that position
				for {
					currentCount := atomic.LoadInt64(&ua.recCount)
					_, exists := ua.serviceRegistry[int(currentCount)]
					if !exists {
						atomic.StoreInt64(&ua.recCount, currentCount)
						rec.Id = int(currentCount)
						break
					}
					atomic.AddInt64(&ua.recCount, 1)
				}

				// Update the record
				rec.Id = int(ua.recCount)
				rec.Created = now.Format(time.RFC3339)
				rec.Updated = now.Format(time.RFC3339)
				rec.EndOfValidity = now.Add(time.Duration(rec.RegLife) * time.Second).Format(time.RFC3339)
				log.Printf("The new service %s from system %s has been registered\n", rec.ServiceDefinition, rec.SystemName)
			} else {
				// Validate and update existing record
				_, exists := ua.serviceRegistry[rec.Id]
				if !exists {
					ua.mu.Unlock()
					continue
				}
				dbRec := ua.serviceRegistry[rec.Id]
				if dbRec.ServiceDefinition != rec.ServiceDefinition {
					request.Error <- errors.New("mismatch between definition received record and database record")
					ua.mu.Unlock()
					continue
				}
				if dbRec.SubPath != rec.SubPath {
					request.Error <- errors.New("mismatch between path received record and database record")
					ua.mu.Unlock()
					continue
				}
				recCreated, err := time.Parse(time.RFC3339, rec.Created)
				if err != nil {
					request.Error <- errors.New("time parsing problem with updated record")
					ua.mu.Unlock()
					continue
				}
				dbCreated, err := time.Parse(time.RFC3339, dbRec.Created)
				if err != nil {
					request.Error <- errors.New("time parsing problem with archived record")
					ua.mu.Unlock()
					continue
				}
				if !recCreated.Equal(dbCreated) {
					request.Error <- errors.New("mismatch between created received record and database record")
					ua.mu.Unlock()
					continue
				}
				nextExpiration := now.Add(time.Duration(dbRec.RegLife) * time.Second).Format(time.RFC3339)
				rec.EndOfValidity = nextExpiration
				// log.Printf("Updated the record %s with next expiration date at %s", rec.ServiceDefinition, rec.EndOfValidity)
			}
			ua.sched.AddTask(now.Add(time.Duration(rec.RegLife)*time.Second), func() { checkExpiration(ua, rec.Id) }, rec.Id)
			ua.serviceRegistry[rec.Id] = *rec // Add record to the registry
			request.Record = rec
			ua.mu.Unlock()
			request.Error <- nil // Send success response

		case "read":
			// Handle read records
			if request.Record == nil {
				var result []forms.ServiceRecord_v1
				ua.mu.Lock() // Lock the serviceRegistry map
				for _, record := range ua.serviceRegistry {
					result = append(result, record)
				}
				ua.mu.Unlock() // Unlock access to the service registry map
				request.Result <- result
				// log.Println("complete listing sent from registry")
				continue
			}
			qform, ok := request.Record.(*forms.ServiceQuest_v1)
			if !ok {
				fmt.Println("Problem unpacking the service quest request")
				request.Error <- fmt.Errorf("invalid record type")
				continue
			}
			fmt.Printf("\nThe service quest form is %v\n\n", qform)
			matchingRecords := ua.FilterByServiceDefinitionAndDetails(qform.ServiceDefinition, qform.Details)
			request.Result <- matchingRecords

		case "delete":
			// Handle delete record
			delete(ua.serviceRegistry, int(request.Id))
			if _, exists := ua.serviceRegistry[int(request.Id)]; !exists {
				log.Printf("The service with ID %d has been deleted.", request.Id)
			}
			request.Error <- nil // Send success response
		}
	}
}

// FilterByServiceDefinitionAndDetails returns a list of services with the given service definition and details TODO: protocols
func (ua *UnitAsset) FilterByServiceDefinitionAndDetails(desiredDefinition string, requiredDetails map[string][]string) []forms.ServiceRecord_v1 {
	ua.mu.Lock() // Ensure thread safety
	defer ua.mu.Unlock()

	var matchingRecords []forms.ServiceRecord_v1

	for _, record := range ua.serviceRegistry {
		if record.ServiceDefinition == desiredDefinition {
			matchesAllDetails := true

			// Check if all required details match
			for key, values := range requiredDetails {
				recordValues, exists := record.Details[key]
				if !exists {
					matchesAllDetails = false
					break
				}

				// Ensure at least one value in requiredDetails matches record.Details
				valueMatch := false
				for _, requiredValue := range values {
					for _, recordValue := range recordValues {
						if recordValue == requiredValue {
							valueMatch = true
							break
						}
					}
					if valueMatch {
						break
					}
				}

				if !valueMatch {
					matchesAllDetails = false
					break
				}
			}

			if matchesAllDetails {
				matchingRecords = append(matchingRecords, record)
			}
		}
	}

	return matchingRecords
}

// checkExpiration checks if a service has expired and deletes it if it has.
func checkExpiration(ua *UnitAsset, servId int) {
	dbRec := ua.serviceRegistry[servId]
	expiration, err := time.Parse(time.RFC3339, dbRec.EndOfValidity)
	if err != nil {
		log.Printf("time parsing problem when checking service expiration")
		return
	}

	if time.Now().After(expiration) {
		if _, exists := ua.serviceRegistry[servId]; !exists {
			return
		}
		delete(ua.serviceRegistry, int(servId))
		if _, exists := ua.serviceRegistry[servId]; !exists {
			log.Printf("The service with ID %d has been deleted because it was not renewed.", servId)
		}
	}
}

// getUniqueSystems populates the list of systems in a local cloud
func getUniqueSystems(ua *UnitAsset) (*forms.SystemRecordList_v1, error) {
	uniqueSystems := make(map[string]struct{}) // to ensure uniqueness
	var systemList []string                    // final list of unique systems

	ua.mu.Lock() // Ensure thread safety
	defer ua.mu.Unlock()

	for _, record := range ua.serviceRegistry {
		var sAddress string

		// Check for HTTPS
		if port, exists := record.ProtoPort["https"]; exists && port != 0 {
			sAddress = "https://" + record.IPAddresses[0] + ":" + strconv.Itoa(port) + "/" + record.SystemName
		} else if port, exists := record.ProtoPort["http"]; exists && port != 0 { // Check for HTTP
			sAddress = "http://" + record.IPAddresses[0] + ":" + strconv.Itoa(port) + "/" + record.SystemName
		} else {
			fmt.Printf("Warning: %s cannot be modeled\n", record.SystemName)
			continue
		}

		// Ensure uniqueness
		if _, added := uniqueSystems[sAddress]; !added {
			uniqueSystems[sAddress] = struct{}{}
			systemList = append(systemList, sAddress)
		}
	}
	return &forms.SystemRecordList_v1{
		List:    systemList,
		Version: "SystemRecordList_v1",
	}, nil
}
