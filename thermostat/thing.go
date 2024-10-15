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
	"fmt"
	"log"
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
	sigIn usecases.RSignal
	// sigFilt float64
	// sigErr  float64
	sigOut    usecases.RSignal
	jitter    time.Duration
	Setpt     float64       `json:"setpoint"`
	Period    time.Duration `json:"samplingPeriod"`
	Kp        float64       `json:"kp"`
	Lambda    float64       `json:"lamda"`
	Ki        float64       `json:"ki"`
	previousT float64
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
	setPointService := components.Service{
		Definition:  "setpoint",
		SubPath:     "setpoint",
		Details:     map[string][]string{"Unit": {"Celcius"}, "Forms": {"SignalA_v1a"}},
		RegPeriod:   120,
		CUnit:       "Eur/kWh",
		Description: "provides the current thermal setpoint (GET) or sets it (PUT)",
	}
	thermalErrorService := components.Service{
		Definition:  "thermalerror",
		SubPath:     "thermalerror",
		Details:     map[string][]string{"Unit": {"Celcius"}, "Forms": {"SignalA_v1a"}},
		RegPeriod:   120,
		Description: "provides the current difference between the setpoint and the temperature (GET)",
	}
	jitterService := components.Service{
		Definition:  "jitter",
		SubPath:     "jitter",
		Details:     map[string][]string{"Unit": {"millisecond"}, "Forms": {"SignalA_v1a"}},
		RegPeriod:   120,
		Description: "provides the current jitter or control algorithm execution calculated every period (GET)",
	}

	// var uat components.UnitAsset // this is an interface, which we then initialize
	uat := &UnitAsset{
		Name:    "controller_1",
		Details: map[string][]string{"Location": {"Kitchen"}},
		Setpt:   20,
		Period:  10,
		Kp:      5,
		Lambda:  0.5,
		Ki:      0,
		ServicesMap: components.Services{
			setPointService.SubPath:     &setPointService,
			thermalErrorService.SubPath: &thermalErrorService,
			jitterService.SubPath:       &jitterService,
		},
	}
	return uat
}

//-------------------------------------Instatiate the unit assets based on configuration

// newResource creates the Resource resource with its pointers and channels based on the configuration using the tConig structs
func newResource(uac UnitAsset, sys *components.System, servs []components.Service) (components.UnitAsset, func()) {
	// deterimine the protocols that the system supports
	sProtocols := components.SProtocols(sys.Husk.ProtoPort)
	// instantiate the consumed services
	t := &components.Cervice{
		Name:   "temperature",
		Protos: sProtocols,
		Url:    make([]string, 0),
	}

	r := &components.Cervice{
		Name:   "rotation",
		Protos: sProtocols,
		Url:    make([]string, 0),
	}
	// intantiate the unit asset
	ua := &UnitAsset{
		Name:        uac.Name,
		Owner:       sys,
		Details:     uac.Details,
		ServicesMap: components.CloneServices(servs),
		Setpt:       uac.Setpt,
		Period:      uac.Period,
		Kp:          uac.Kp,
		Lambda:      uac.Lambda,
		Ki:          uac.Ki,
		CervicesMap: components.Cervices{
			t.Name: t,
			r.Name: r,
		},
	}
	ua.sigIn.Unit = "Celsius"
	ua.sigIn.QuestForm = usecases.FillQuestForm(sys, ua, "temperature", "http")
	ua.sigIn.Sys = sys
	fmt.Printf("the desired service is: %v+\n", ua.sigIn)
	ua.sigOut.Unit = "Percent"
	ua.sigOut.QuestForm = usecases.FillQuestForm(sys, ua, "rotation", "http")
	ua.sigOut.Sys = sys
	var ref components.Service
	for _, s := range servs {
		if s.Definition == "setpoint" {
			ref = s
		}
	}
	// ua.tunit = ref.Details["unit"][0]
	ua.CervicesMap["temperature"].Details = components.MergeDetails(ua.Details, ref.Details)
	ua.CervicesMap["rotation"].Details = components.MergeDetails(ua.Details, map[string][]string{"Unit": {"Percent"}, "Forms": {"SignalA_v1a"}})
	fmt.Printf("\nThe consumed services are %v+\n", ua.CervicesMap["temperature"])
	fmt.Printf("\nThe unit asset is: %v+/\n", ua)

	// start the unit asset(s)
	go ua.feedbackLoop(sys.Ctx)

	return ua, func() {
		log.Println("Shutting down thermostat ", ua.Name)
	}
}

//-------------------------------------Thing's resource methods

// getSetPoint fills out a signal form with the current thermal setpoint
func (ua *UnitAsset) getSetPoint() (f forms.SignalA_v1a) {
	f.NewForm()
	f.Value = ua.Setpt
	f.Unit = "Celcius"
	f.Timestamp = time.Now()
	return f
}

// setSetPoint updates the thermal setpoint
func (ua *UnitAsset) setSetPoint(f forms.SignalA_v1a) {
	ua.Setpt = f.Value
	log.Printf("new set point: %.1f", f.Value)
}

// getErrror fills out a signal form with the currrent thermal setpoint and temperature
func (ua *UnitAsset) getError() (f forms.SignalA_v1a) {
	f.NewForm()
	f.Value = ua.Setpt - ua.sigIn.Value
	f.Unit = "Celsius"
	f.Timestamp = time.Now()
	return f
}

// getJitter fills out a signal form with the current jitter
func (ua *UnitAsset) getJitter() (f forms.SignalA_v1a) {
	f.NewForm()
	f.Value = float64(ua.jitter.Milliseconds())
	f.Unit = "millisecond"
	f.Timestamp = time.Now()
	return f
}

// feedbackLoop is THE control loop (IPR of the system)
func (ua *UnitAsset) feedbackLoop(ctx context.Context) {
	// Initialize a ticker for periodic execution
	ticker := time.NewTicker(ua.Period * time.Second)
	defer ticker.Stop()

	// start the control loop
	for {
		select {
		case <-ticker.C:
			ua.processFeedbackLoop()
		case <-ctx.Done():
			return
		}
	}
}

// processFeedbackLoop is called to execute the control process
func (ua *UnitAsset) processFeedbackLoop() {
	jitterStart := time.Now()

	// get the current temperature
	tf, err := usecases.GetState(ua.CervicesMap["temperature"], ua.Owner)
	if err != nil {
		fmt.Printf("\n We have a getState error: %s", err)
	}
	// Perform a type assertion to convert the returned Form to SignalA_v1a
	tup, ok := tf.(*forms.SignalA_v1a)
	if !ok {
		fmt.Println("Problem unpacking the service discovery request form")
		return
	}

	// perform the control algorithm
	deviation := ua.Setpt - tup.Value
	output := ua.calculateOutput(deviation)

	// prepare the form to send
	var of forms.SignalA_v1a
	of.NewForm()
	of.Value = output
	of.Unit = ua.CervicesMap["rotation"].Details["Unit"][0]
	of.Timestamp = time.Now()

	// pack the new valve state form
	op, err := usecases.Pack(&of, "application/json")
	if err != nil {
		return
	}
	// send the new valve state request
	err = usecases.SetState(ua.CervicesMap["rotation"], ua.Owner, op)
	if err != nil {
		fmt.Printf("thermostat-feedback: could not update output signal: %s\n", err)
		return
	}

	if tup.Value != ua.previousT {
		log.Printf("Temperature %.2f °C with error %.2f°C and actuator at %.2f%%\n", tup.Value, deviation, output)
		ua.previousT = tup.Value
	}

	ua.jitter = time.Since(jitterStart)
}

// calculateOutput is the actual P contoroller (no real close loop yet)
func (ua *UnitAsset) calculateOutput(thermDiff float64) float64 {
	vPosition := ua.Kp*thermDiff + 50 // if the error is 0, the position is at 50%

	// limit the output between 0 and 100%
	if vPosition < 0 {
		vPosition = 0
	} else if vPosition > 100 {
		vPosition = 100
	}
	return vPosition
}
