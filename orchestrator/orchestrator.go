/*******************************************************************************
 * Copyright (c) 2023 Jan van Deventer
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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
	"github.com/sdoque/mbaigo/usecases"
)

func main() {
	// prepare for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background()) // create a context that can be cancelled
	defer cancel()                                          // make sure all paths cancel the context to avoid context leak

	// instantiate the System
	sys := components.NewSystem("orchestrator", ctx)

	// Instatiate the Capusle
	sys.Husk = &components.Husk{
		Description: "provides the URL of a currently available and authorized sought service",
		Certificate: "ABCD",
		Details:     map[string][]string{"Developer": {"Arrowhead"}},
		ProtoPort:   map[string]int{"https": 0, "http": 8445, "coap": 0},
		InfoLink:    "https://github.com/sdoque/systems/tree/master/orchestrator",
	}

	// instantiate a template unit asset
	assetTemplate := initTemplate()
	assetName := assetTemplate.GetName()
	sys.UAssets[assetName] = &assetTemplate

	// Configure the system
	rawResources, servsTemp, err := usecases.Configure(&sys)
	if err != nil {
		log.Fatalf("Configuration error: %v\n", err)
	}
	sys.UAssets = make(map[string]*components.UnitAsset) // clear the unit asset map (from the template)
	for _, raw := range rawResources {
		var uac UnitAsset
		if err := json.Unmarshal(raw, &uac); err != nil {
			log.Fatalf("Resource configuration error: %+v\n", err)
		}
		ua, cleanup := newResource(uac, &sys, servsTemp)
		defer cleanup()
		sys.UAssets[ua.GetName()] = &ua
	}

	// Generate PKI keys and CSR to obtain a authentication certificate from the CA
	usecases.RequestCertificate(&sys)

	// Register the (system) and its services
	usecases.RegisterServices(&sys)

	// start the http handler and server
	go usecases.SetoutServers(&sys)

	// wait for shutdown signal, and gracefully close properly goroutines with context
	<-sys.Sigs // wait for a SIGINT (Ctrl+C) signal
	fmt.Println("\nshuting down system", sys.Name)
	cancel()                    // cancel the context, signaling the goroutines to stop
	time.Sleep(3 * time.Second) // allow the go routines to be executed, which might take more time than the main routine to end
}

// Serving handles the resources services. NOTE: it exepcts those names from the request URL path
func (t *UnitAsset) Serving(w http.ResponseWriter, r *http.Request, servicePath string) {
	switch servicePath {
	case "squest":
		t.orchestrate(w, r)

	default:
		http.Error(w, "Invalid service request [Do not modify the services subpath in the configurration file]", http.StatusBadRequest)
	}
}

func (ua *UnitAsset) orchestrate(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		defer r.Body.Close()
		bodyBytes, err := io.ReadAll(r.Body) // Use io.ReadAll instead of ioutil.ReadAll
		if err != nil {
			log.Printf("error reading discovery request body: %v\n", err)
			return
		}
		headerContentTtype := r.Header.Get("Content-Type")
		questForm, err := usecases.Unpack(bodyBytes, headerContentTtype)
		if err != nil {
			log.Printf("error extracting the discovery request %v\n", err)
		}
		// Perform a type assertion to convert the returned Form to SignalA_v1a
		qf, ok := questForm.(*forms.ServiceQuest_v1)
		if !ok {
			fmt.Println("Problem unpacking the service discovery request form")
			return
		}
		fmt.Printf("The service discovery request form is %v\n", qf)

		// questForm, err := usecases.ExtractQuestForm(bodyBytes)
		if err != nil {
			log.Printf("error extracting the discovery request %v\n", err)
		}
		servLocation, err := ua.getServiceURL(*qf)
		if err != nil {
			fmt.Printf("we have a problem locating the service, %v\n", err)
		}
		fmt.Printf("The service location is %v\n", servLocation)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(servLocation) // respond with the selected servicelocation
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "Method is not supported.", http.StatusNotFound)
	}
}
