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
	"fmt"
	"log"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
	"github.com/stianeikeland/go-rpio/v4"
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
	GpioPin  int      `json:"gpiopin"`
	rPi_Pin  rpio.Pin `json:"-"`
	position int      `json:"-"`
	dutyChan chan int `json:"-"`
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
	rotation := components.Service{
		Definition:  "rotation",
		SubPath:     "rotation",
		Details:     map[string][]string{"Forms": {"SignalA_v1a"}, "Unit": {"percent", "rotational"}},
		RegPeriod:   30,
		Description: "informs of the servo's current postion (GET) or updates the position (PUT)",
	}

	// var uat components.UnitAsset // this is an interface, which we then initialize
	uat := &UnitAsset{
		Name:    "Servo_1",
		Details: map[string][]string{"Model": {"standard servo", "-90 to +90 degrees"}, "Location": {"Kitchen"}},
		ServicesMap: components.Services{
			rotation.SubPath: &rotation, // Inline assignment of the temperature service
		},
		GpioPin: 18,
	}
	return uat
}

//-------------------------------------Instatiate the unit assets based on configuration

// newResource creates the Resource resource with its pointers and channels based on the configuration using the tConig structs
func newResource(uac UnitAsset, sys *components.System, servs []components.Service) (components.UnitAsset, func()) {
	// ua components.UnitAsset is an interface, which is implemneted and initialized
	ua := &UnitAsset{
		Name:        uac.Name,
		Owner:       sys,
		Details:     uac.Details,
		ServicesMap: components.CloneServices(servs),
		GpioPin:     uac.GpioPin,
		dutyChan:    make(chan int),
	}
	// Initialize the GPIO pin
	if err := rpio.Open(); err != nil {
		log.Fatalf("Failed to open GPIO %s\n", err)
		return ua, func() {

		}
	}
	ua.rPi_Pin = rpio.Pin(ua.GpioPin)
	ua.rPi_Pin.Output()
	ua.rPi_Pin.Mode(rpio.Pwm)
	ua.rPi_Pin.Freq(1_000_000)        // µs in one s
	ua.rPi_Pin.DutyCycle(620, 20_000) // 0°

	// start the unit asset(s)
	// Start a single goroutine to update ua.temperature from ua.TempChan
	go func() {
		for pulseWidth := range ua.dutyChan {
			fmt.Printf("Pulse width updated: %v\n", pulseWidth)
			ua.rPi_Pin.DutyCycle(uint32(pulseWidth), 20_000) // Adjusting to the new pulse width
		}
	}()

	return ua, func() {
		log.Println("disconnecting from servos")
		rpio.Close()
	}
}

//-------------------------------------Unit asset's resource functions

// timing constants for the PWM (pulse width modulation)
// pulse widths:(620 µs, 1520 µs, 2420 µs) maps to (0°, 90°, 180°) with angles increasing from clockwise to counterclockwise
const (
	minPulseWidth    = 620
	centerPulseWidth = 1520
	maxPulseWidth    = 2420
)

// getPosition provides an analog signal form fit the srevo position in percent and a timestamp
func (ua *UnitAsset) getPosition() (f forms.SignalA_v1a) {
	f.NewForm()
	f.Value = float64(ua.position)
	f.Unit = "percent"
	f.Timestamp = time.Now()
	return f
}

// setPosition update the PWM pulse size based on the requested position [0-100]%
func (ua *UnitAsset) setPosition(f forms.SignalA_v1a) {
	fmt.Printf("The new position is %+v\n", f)

	// Limit the value directly within the assignment to rsc.position
	position := int(f.Value)
	if position < 0 {
		position = 0
	} else if position > 100 {
		position = 100
	}
	ua.position = position // Position is now guaranteed to be in the 0-100 % range

	// Calculate the width based on the position, scaled to pulse width range
	width := (ua.position * (maxPulseWidth - minPulseWidth) / 100) + minPulseWidth

	// Send the calculated width to the duty cycle channel
	ua.dutyChan <- width
}
