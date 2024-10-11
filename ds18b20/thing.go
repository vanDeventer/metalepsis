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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

//-------------------------------------Define the unit asset

// UnitAsset type models the unit asset (interface) of the system.
type UnitAsset struct {
	Name        string              `json:"name"`
	Owner       *components.System  `json:"-"`
	Details     map[string][]string `json:"details"`
	ServicesMap components.Services `json:"-"`
	CervicesMap components.Cervices `json:"-"`
	//
	temperature float64      `json:"-"`
	TempChan    chan float64 `json:"-"` // Add a channel for temperature readings
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
	temperature := components.Service{
		Definition:  "temperature",
		SubPath:     "temperature",
		Details:     map[string][]string{"Forms": {"SignalA_v1a"}},
		RegPeriod:   30,
		Description: "provides the temperature (GET) of the resource temperature sensor",
	}

	// var uat components.UnitAsset // this is an interface, which we then initialize
	uat := &UnitAsset{
		Name:    "sensor_Id",
		Details: map[string][]string{"Unit": {"Celsius"}, "Location": {"Kitchen"}},
		ServicesMap: components.Services{
			temperature.SubPath: &temperature, // Inline assignment of the temperature service
		},
	}
	return uat
}

//-------------------------------------Instatiate the unit assets based on configuration

// newResource creates the Resource resource with its pointers and channels based on the configuration
func newResource(uac UnitAsset, sys *components.System, servs []components.Service) (components.UnitAsset, func()) {
	ua := &UnitAsset{ // this a struct that implements the UnitAsset interface
		Name:        uac.Name,
		Owner:       sys,
		Details:     uac.Details,
		ServicesMap: components.CloneServices(servs),
		TempChan:    make(chan float64, 1), // Initialize the channel with a buffer
	}

	// start the unit asset(s)
	// Start a single goroutine to update ua.temperature from ua.TempChan
	go func() {
		for temp := range ua.TempChan {
			// here could use synchronization mechanisms if multiple goroutines access ua.temperature
			ua.temperature = temp
		}
	}()
	go ua.readTemperature(sys.Ctx)
	// }

	return ua, func() {
		log.Println("disconnecting from sensors")
	}
}

//-------------------------------------Unit asset's resource functions

// readTemperature obtains the temperature from respective ds18b20 resource at regular intervals
func (ua *UnitAsset) readTemperature(ctx context.Context) {
	// the responsible channel writing routine closes the channel when exiting
	defer close(ua.TempChan)

	// Initialize the timer outside the loop for the first delay
	timer := time.NewTimer(2 * time.Second)
	// ensure the timer is stopped to avoid resource leak
	defer timer.Stop()

	for {
		// Wait for the timer or context cancellation
		select {
		case <-ctx.Done():
			return // exit the goroutine if the context is done
		case <-timer.C: // Wait for the timer to fire

			// path to the DS18B20 sensor device file on a Raspberry Pi
			deviceFile := "/sys/bus/w1/devices/" + ua.Name + "/w1_slave"

			// Read the raw temperature value from the device file
			var rawData []byte
			var err error
			// keep trying to read until rawData is not empty
			for {
				rawData, err = os.ReadFile(deviceFile)
				if err != nil {
					fmt.Printf("Error reading temperature file: %s, error: %v\n", deviceFile, err)
					return
				}

				if len(rawData) > 0 {
					break // exit the loop if data is successfully read
				}
				fmt.Printf("Empty data read from temperature file: %s, retrying...\n", deviceFile)
				time.Sleep(1 * time.Second) // wait before retrying
			}

			// parse the raw temperature value
			rawValue := strings.Split(string(rawData), "\n")[1]
			if !strings.Contains(rawValue, "t=") {
				fmt.Println("Invalid temperature data: %", rawData)
				return // and exit the goroutine
			}
			tempStr := strings.Split(rawValue, "t=")[1]
			temp, err := strconv.ParseFloat(tempStr, 64)
			if err != nil {
				fmt.Println("Error reading temperature:", err)
				return // and exit the goroutine
			}
			select {
			case ua.TempChan <- temp / 1000.0: // send temperature reading to the channel
			case <-ctx.Done():
				return // exit if the context is cancelled
			}
		}

		timer.Reset(2 * time.Second) // Reset the timer for the next read
	}
}

// serveTemperature fills out a signal form with the value of temperature of the sensor (but does not check if it is functional)
func serveTemperature(sensor *UnitAsset) (f forms.SignalA_v1a) {

	f.NewForm()
	// Convert the raw temperature value to degrees Celsius
	f.Value = sensor.temperature
	f.Unit = "Celcius"
	f.Timestamp = time.Now()
	return f
}
