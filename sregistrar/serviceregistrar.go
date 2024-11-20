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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	sys := components.NewSystem("serviceregistrar", ctx)

	// Instatiate the Capusle
	sys.Husk = &components.Husk{
		Description: "is an Arrowhead mandatory core sysstem that keeps track of the currently available sevices.",
		Details:     map[string][]string{"Developer": {"Arrowhead"}},
		ProtoPort:   map[string]int{"https": 0, "http": 8443, "coap": 0},
		InfoLink:    "https://github.com/sdoque/systems/tree/master/serviceregistrar",
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
	//	Resources := make(map[string]*UnitAsset)
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

// ---------------------------------------------------------------------------- end of main()

// Serving handles the resources services. NOTE: it exepcts those names from the request URL path
func (ua *UnitAsset) Serving(w http.ResponseWriter, r *http.Request, servicePath string) {
	switch servicePath {
	case "register":
		ua.updateDB(w, r)
	case "query":
		ua.queryDB(w, r)
	case "unregister":
		ua.cleanDB(w, r)
	case "status":
		ua.roleStatus(w, r)
	case "syslist":
		ua.systemList(w, r)
	default:
		http.Error(w, "Invalid service request [Do not modify the services subpath in the configurration file]", http.StatusBadRequest)
	}
}

// updateDB is used to add a new service record or to extend its registration life
func (ua *UnitAsset) updateDB(w http.ResponseWriter, r *http.Request) {
	if !ua.leading {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Service Unavailable"))
		return
	}
	switch r.Method {
	case "POST", "PUT":
		contentType := r.Header.Get("Content-Type")
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			fmt.Println("Error parsing media type:", err)
			return
		}
		defer r.Body.Close()
		bodyBytes, err := io.ReadAll(r.Body) // Use io.ReadAll instead of ioutil.ReadAll
		if err != nil {
			log.Printf("error reading registration request body: %v", err)
			return
		}

		regRec, err := usecases.Unpack(bodyBytes, mediaType)
		if err != nil {
			log.Printf("error extracting the registration record relpy %v\n", err)
		}

		// Perform a type assertion to convert the returned Form to ServiceRecord_v1
		newRecord, ok := regRec.(*forms.ServiceRecord_v1)
		if !ok {
			fmt.Println("error extracting registration request")
			return
		}

		// Process request ////////////////////////////////////////////////////

		if newRecord.Id == 0 {
			err = registerService(ua, newRecord) // insert the new record into the database
			log.Printf("the new service %s from system %s has been registered\n", newRecord.ServiceDefinition, newRecord.SystemName)
			if err != nil {
				log.Println(err)
			}
		} else {
			err = extendServiceValidity(ua, newRecord)
			if err != nil {
				err = registerService(ua, newRecord) // insert the new record into the database since the "existing" record was not found
				log.Printf("the service %s from system %s has been re-registered\n", newRecord.ServiceDefinition, newRecord.SystemName)
				if err != nil {
					log.Println(err)
				}
			}
		}

		jform, err := usecases.Pack(newRecord, mediaType)
		if err != nil {
			log.Println("registration marshall error")
		}
		w.Header().Set("Content-Type", mediaType)
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(jform)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		fmt.Fprintf(w, "unsupported http request method")
	}
}

// queryDB looks for service records in the service registry
func (ua *UnitAsset) queryDB(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// Handle GET request - no payload, only URL query parameters
		serviceList := listCurrentServices(ua)
		text := "<!DOCTYPE html><html><body>"
		w.Write([]byte(text))
		text = "<p>The local cloud's currently available services are:</p><ul>"
		w.Write([]byte(text))
		for _, availableService := range serviceList {
			w.Write([]byte(fmt.Sprintf("<li>%s</li>", availableService)))
		}
		text = "</ul></body></html>"
		w.Write([]byte(text))

	case "POST":
		// Handle POST request - with a JSON payload from the Orchestrator
		headerContentType := r.Header.Get("Content-Type")
		if !strings.Contains(headerContentType, "application/json") {
			http.Error(w, "Unsupported Media Type", http.StatusUnsupportedMediaType)
			return
		}

		defer r.Body.Close()
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Error reading service query request body: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		questForm, err := usecases.Unpack(bodyBytes, headerContentType)
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

		// Process request and get a copy of the availavle services in a list of ServiceRecords
		discoveryList, err := findServices(ua, *qf)
		if err != nil {
			log.Printf("Error querying the Service Registry: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// fill out the form that has the list of services that fit the request
		dsListForm, err := usecases.FillDiscoveredServices(discoveryList, "ServiceRecordList_v1")
		if err != nil {
			log.Println("service record processing error")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// package up the list into a byte array
		payload, err := usecases.Pack(dsListForm, headerContentType)
		if err != nil {
			log.Println("Discovery marshalling error")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		fmt.Printf("The list of discovered services is %v+\n", dsListForm)

		// send off the list back to the Orchestrator
		w.Header().Set("Content-Type", headerContentType)
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	default:
		http.Error(w, "Unsupported HTTP request method", http.StatusMethodNotAllowed)
	}
}

// cleanDB deletes service records upon request (e.g., when a system shuts down)
func (ua *UnitAsset) cleanDB(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "DELETE":
		parts := strings.Split(r.URL.Path, "/")
		idStr := parts[len(parts)-1]   // the ID is the last part of the URL path
		id, err := strconv.Atoi(idStr) // convert the ID to an integer
		if err != nil {
			// handle the error
			http.Error(w, "Invalid record ID", http.StatusBadRequest)
			return
		}
		deleteCompleteServiceById(ua, id)
		if !ua.sched.RemoveTask(id) {
			log.Printf("the scheduler had no task with id %d to remove", id)
		}
	default:
		fmt.Fprintf(w, "unsupported http request method")
	}
}

// roleStatus rerturn the current activity of a service registrar (i.e., leading or on stand by)
func (ua *UnitAsset) roleStatus(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		if ua.leading {
			text := fmt.Sprintf("lead Service Registrar since %s", ua.leadingSince)
			fmt.Fprint(w, text)
			return
		}
		if ua.leadingRegistrar != nil {
			text := fmt.Sprintf("On standby, leading registrar is %s", ua.leadingRegistrar.Url)
			http.Error(w, text, http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Service Unavailable"))
	default:
		fmt.Fprintf(w, "unsupported http request method")
	}
}

// Role repeatedly check which service registrar in the local cloud is the leading service registrar
func (ua *UnitAsset) Role() {
	peersList, err := peersList(ua.Owner)
	if err != nil {
		panic(err)
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		for {
			standby := false
		foundLead:
			for _, cSys := range peersList {
				resp, err := http.Get(cSys.Url + "/status")
				if err != nil {
					break // that system registrar is not up
				}
				defer resp.Body.Close()

				// Handle status codes
				switch resp.StatusCode {
				case http.StatusOK:
					standby = true
					ua.leading = false
					ua.leadingSince = time.Time{} // reset lead timer
					ua.leadingRegistrar = cSys
					break foundLead
				case http.StatusServiceUnavailable:
					// Service unavailable
				default:
					fmt.Printf("Received unexpected status code: %d\n", resp.StatusCode)
				}
			}
			if !standby && !ua.leading {
				ua.leading = true
				ua.leadingSince = time.Now()
				ua.leadingRegistrar = nil
				fmt.Printf("taking the service registry lead at %s\n", ua.leadingSince)
			}
			<-ticker.C
		}
	}()
}

// peerslist provides a list of the other service registrars in the local cloud
func peersList(sys *components.System) (peers []*components.CoreSystem, err error) {
	for _, cs := range sys.CoreS {
		if cs.Name != "serviceregistrar" {
			continue
		}
		u, err := url.Parse(cs.Url)
		if err != nil {
			return peers, err
		}
		uPort, err := strconv.Atoi(u.Port())
		if err != nil {
			fmt.Println(err)
		}
		if (u.Hostname() == sys.Host.IPAddresses[0] || u.Hostname() == "localhost") && uPort == sys.Husk.ProtoPort[u.Scheme] {
			continue
		}
		peers = append(peers, cs)
	}
	return peers, nil
}

// queryDB looks for service records in the service registry
func (ua *UnitAsset) systemList(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		systemsList, err := getUniqueSystems(ua)
		if err != nil {
			fmt.Printf("system list error, %s", err)
		}
		usecases.HTTPProcessGetRequest(w, r, systemsList)
	default:
		fmt.Fprintf(w, "unsupported http request method")
	}
}
