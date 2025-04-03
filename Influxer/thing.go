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
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
	"github.com/sdoque/mbaigo/usecases"
)

// -------------------------------------Define a measurement (or signal)
type MeasurementT struct {
	Name    string              `json:"serviceDefinition"`
	Details map[string][]string `json:"mdetails"`
	Period  time.Duration       `json:"samplingPeriod"`
}

//-------------------------------------Define the unit asset

// UnitAsset type models the unit asset (interface) of the system
type UnitAsset struct {
	Name        string              `json:"bucket_name"`
	Owner       *components.System  `json:"-"`
	Details     map[string][]string `json:"details"`
	ServicesMap components.Services `json:"-"`
	CervicesMap components.Cervices `json:"-"`
	//
	FluxURL      string           `json:"db_url"`
	Token        string           `json:"token"`
	Org          string           `json:"organization"`
	Bucket       string           `json:"bucket"`
	Measurements []MeasurementT   `json:"measurements"`
	client       influxdb2.Client // InfluxDB client
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
	mqueryService := components.Service{
		Definition:  "mquery",
		SubPath:     "mquery",
		Details:     map[string][]string{},
		RegPeriod:   60,
		CUnit:       "",
		Description: "provides the list of measurements in the bucket (GET)",
	}

	uat := &UnitAsset{
		Name:    "demo",
		Details: map[string][]string{"Database": {"InfluxDB"}},
		FluxURL: "http://10.0.0.33:8086",
		Token:   "K1NTWNlToyUNXdii7IwNJ1W-kMsagUr8w1r4cRVYqK-N-R9vVT1MCJwHFBxOgiW85iKiMSsUpbrxQsQZJA8IzA==",
		Org:     "mbaigo",
		Bucket:  "demo",
		Measurements: []MeasurementT{
			{
				Name:    "temperature",
				Details: map[string][]string{"Location": {"Kitchen"}},
				Period:  3,
			},
		},
		ServicesMap: components.Services{
			mqueryService.SubPath: &mqueryService,
		},
	}
	return uat
}

//-------------------------------------Instantiate the unit assets based on configuration

// newResource creates a new UnitAsset resource based on the configuration
func newResource(uac UnitAsset, sys *components.System, servs []components.Service) (components.UnitAsset, func()) {
	ua := &UnitAsset{
		Name:        uac.Name,
		Owner:       sys,
		Details:     uac.Details,
		ServicesMap: components.CloneServices(servs),
		FluxURL:     uac.FluxURL,
		Token:       uac.Token,
		Org:         uac.Org,
		Bucket:      uac.Bucket,
		CervicesMap: make(map[string]*components.Cervice), // Initialize map
	}

	if ua.FluxURL == "" || ua.Token == "" || ua.Org == "" || ua.Bucket == "" {
		log.Fatal("Invalid InfluxDB configuration: missing required parameters")
	}

	// Create a new client for InfluxDB
	ua.client = influxdb2.NewClient(ua.FluxURL, ua.Token)

	// Create a non-blocking write API
	writeAPI := ua.client.WriteAPI(ua.Org, ua.Bucket)

	// Collect and ingest measurements
	var wg sync.WaitGroup
	for _, measurement := range uac.Measurements {
		cMeasurement := components.Cervice{
			Name:    measurement.Name,
			Details: measurement.Details,
			Url:     make([]string, 0),
		}
		ua.CervicesMap[cMeasurement.Name] = &cMeasurement

		wg.Add(1)
		go func(name string, period time.Duration) {
			defer wg.Done()
			if err := ua.collectIngest(name, period, writeAPI); err != nil {
				log.Printf("Error in collectIngest for measurement: %v", err)
			}
		}(measurement.Name, measurement.Period)
	}

	// Return the unit asset and a cleanup function to close the InfluxDB client
	return ua, func() {
		log.Println("Waiting for all goroutines to finish...")
		wg.Wait()
		log.Println("Disconnecting from InfluxDB")
		ua.client.Close()
	}
}

//-------------------------------------Unit asset's functionalities

// collectIngest
func (ua *UnitAsset) collectIngest(name string, period time.Duration, writeAPI api.WriteAPI) error {
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-ua.Owner.Ctx.Done():
			log.Printf("Stopping data collection for measurement: %s", name)
			return ua.Owner.Ctx.Err()

		case <-ticker.C:
			tf, err := usecases.GetState(ua.CervicesMap[name], ua.Owner)
			if err != nil {
				log.Printf("\nUnable to obtain a %s reading with error %s\n", name, err)
				continue // return fmt.Errorf("unsupported measurement: %s", name)
			}

			// Perform a type assertion to convert the returned Form to SignalA_v1a
			tup, ok := tf.(*forms.SignalA_v1a)
			if !ok {
				log.Println("Problem unpacking the signal form")
				continue // return fmt.Errorf("problem unpacking measurement: %s", name)
			}

			metaD := ua.CervicesMap[name].Details

			// Convert metaD (map[string][]string) into InfluxDB tags (map[string]string)
			tags := make(map[string]string)
			for key, values := range metaD {
				// Join all values in the slice with a comma
				tags[key] = strings.Join(values, ",")
			}

			// Create an InfluxDB point using metaD as tags
			point := write.NewPoint(
				name,
				tags, // Transformed metaD as tags
				map[string]interface{}{"value": tup.Value}, // Field value
				time.Now(), // Timestamp
			)

			// Write point to InfluxDB using WriteAPI
			writeAPI.WritePoint(point)
		}
	}
}

// q4measurements queries the bucket for the list of measurements
func (ua *UnitAsset) q4measurements(w http.ResponseWriter) {
	text := "The list of measurements in the " + ua.Name + " bucket is:\n"
	queryAPI := ua.client.QueryAPI(ua.Org)

	query := fmt.Sprintf(`
		 import "influxdata/influxdb/schema"
		 schema.measurements(bucket: "%s")
	 `, ua.Name)

	results, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		log.Fatal(err)
	}

	for results.Next() {
		measurement := fmt.Sprintf("%v", results.Record().Value())
		text += "- " + measurement + "\n"
	}

	if err := results.Err(); err != nil {
		log.Fatal(err)
	}

	w.Write([]byte(text))
}
