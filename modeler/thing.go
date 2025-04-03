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
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
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
	SystemList    forms.SystemRecordList_v1 `json:"-"`
	RepositoryURL string                    `json:"repositoryURL"`
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
	model := components.Service{
		Definition:  "model",
		SubPath:     "model",
		Details:     map[string][]string{"Format": {"Turtle"}},
		RegPeriod:   61,
		Description: "provides the semantic model of a local cloud (GET)",
	}

	// var uat components.UnitAsset // this is an interface, which we then initialize
	uat := &UnitAsset{
		Name:          "assembler",
		Owner:         &components.System{},
		Details:       map[string][]string{"Location": {"Local cloud"}},
		ServicesMap:   map[string]*components.Service{model.SubPath: &model},
		RepositoryURL: "http://localhost:7200/repositories/Arrowhead/statements",
	}
	return uat
}

//-------------------------------------Instantiate unit asset(s) based on configuration

// newResource creates the unit asset with its pointers and channels based on the configuration
func newResource(uac UnitAsset, sys *components.System, servs []components.Service) (components.UnitAsset, func()) {
	// var ua components.UnitAsset // this is an interface, which we then initialize
	ua := &UnitAsset{ // this is an interface, which we then initialize
		Name:          uac.Name,
		Owner:         sys,
		Details:       uac.Details,
		ServicesMap:   components.CloneServices(servs),
		RepositoryURL: uac.RepositoryURL,
	}

	// start the unit asset(s)

	return ua, func() {
		log.Println("Disconnecting from GraphDB")
	}
}

// -------------------------------------Unit asset's function methods

// assembles ontologies gets the list of systems from the lead registrar and then the ontology of each system
func (ua *UnitAsset) assembleOntologies(w http.ResponseWriter) {
	// Look for leading service registrar
	var leadingRegistrar *components.CoreSystem
	for _, cSys := range ua.Owner.CoreS {
		core := cSys
		if core.Name == "serviceregistrar" {
			resp, err := http.Get(core.Url + "/status")
			if err != nil {
				fmt.Println("Error checking service registrar status:", err)
				continue
			}
			bodyBytes, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				fmt.Println("Error reading service registrar response body:", err)
				continue
			}
			if strings.HasPrefix(string(bodyBytes), "lead Service Registrar since") {
				leadingRegistrar = core
			}
		}
	}

	if leadingRegistrar == nil {
		fmt.Printf("no service registrar found\n")
		http.Error(w, "Internal Server Error: no service registrar found", http.StatusInternalServerError)
		return
	}

	// request list of systems in the cloud
	leadUrl := leadingRegistrar.Url + "/syslist"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequest(http.MethodGet, leadUrl, nil)
	if err != nil {
		log.Printf("Error getting the systems list from service registrar, %s\n", err)
		return
	}
	req = req.WithContext(ctx)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error receiving the systems list from service registrar, %s\n", err)
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("GetRValue-Error reading registration response body: %v", err)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		fmt.Println("Error parsing media type:", err)
		return
	}
	sL, err := usecases.Unpack(bodyBytes, mediaType)
	if err != nil {
		log.Printf("error extracting the systems list reply %v\n", err)
		return
	}

	// Perform a type assertion to convert the returned Form to ServiceRecord_v1
	systemsList, ok := sL.(*forms.SystemRecordList_v1)
	if !ok {
		fmt.Println("Problem unpacking the service registration reply")
		return
	}

	// Prepare the local cloud's semantic model by asking each system their semantic model
	prefixes := make(map[string]bool)        // To store unique prefixes
	processedBlocks := make(map[string]bool) // To track processed RDF blocks
	var uniqueIndividuals []string           // To store unique RDF individuals

	for _, s := range systemsList.List {
		sysUrl := s + "/model"
		fmt.Println(sysUrl)
		resp, err := http.Get(sysUrl)
		if err != nil {
			log.Printf("Unable to get ontology from %s: %s\n", s, err)
			continue
		}
		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Error reading ontology response from %s: %s\n", s, err)
			continue
		}

		// Split into individual RDF blocks
		blocks := strings.Split(string(bodyBytes), "\n\n") // Assuming blocks are separated by newlines

		for _, block := range blocks {
			normalizedBlock := strings.TrimSpace(block)
			if processedBlocks[normalizedBlock] {
				// Skip duplicate block
				continue
			}

			// Extract prefixes only from the first pass and add to the prefixes map
			if strings.HasPrefix(normalizedBlock, "@prefix") {
				lines := strings.Split(normalizedBlock, "\n")
				for _, line := range lines {
					if strings.HasPrefix(line, "@prefix") {
						prefixes[line] = true // Add unique prefixes
					}
				}
				continue // Skip adding prefixes as RDF blocks
			}

			// Mark this block as processed and add to individuals
			processedBlocks[normalizedBlock] = true
			uniqueIndividuals = append(uniqueIndividuals, normalizedBlock)
		}
	}

	var graph string

	// Write unique prefixes once
	for prefix := range prefixes {
		graph += prefix + "\n"
	}

	// Add the ontology definition
	rdf := "\n:ontology a owl:Ontology .\n"
	graph += rdf + "\n"

	// Write unique RDF blocks
	for _, block := range uniqueIndividuals {
		graph += block + "\n\n"
	}

	// Send the semantic model to GraphDB
	req, err = http.NewRequest("POST", ua.RepositoryURL, bytes.NewBuffer([]byte(graph)))
	if err != nil {
		fmt.Println("Error creating the request:", err)
		return
	}

	// Set appropriate headers
	req.Header.Set("Content-Type", "text/turtle")

	// Send the request
	client = &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		fmt.Println("Error sending the request:", err)
		return
	}
	defer resp.Body.Close()

	// Read and print the response
	body, _ := io.ReadAll(resp.Body)
	fmt.Println("Response Status:", resp.Status)
	fmt.Println("Response Body:", string(body))

	// Send the knowledge graph to the browser
	w.Header().Set("Content-Type", "text/turtle")
	w.Write([]byte(graph))
}
