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
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
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
	ServerAddress string              `json:"serverAddress"`
	RegisterMap   map[string][]string `json:"register_map"`
	conn          *net.Conn           `json:"-"`
	IOtype        ioType              `json:"-"`
	Address       string              `json:"-"`
	Access        string              `json:"-"`
	DataType      string              `json:"-"`
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
	access := components.Service{
		Definition:  "access",
		SubPath:     "access",
		Details:     map[string][]string{"Protocol": {"tcp"}},
		RegPeriod:   30,
		Description: "accesses the Modbus slave's coil, discrete input, holding and input registers to read (GET) the information or write (PUT), ",
	}

	// var uat components.UnitAsset // this is an interface, which we then initialize
	uat := &UnitAsset{
		Name:          "PLC with Modbus slave",
		Details:       map[string][]string{"PLC": {"Wago"}, "Location": {"A2306"}},
		ServerAddress: "192.168.1.2:502",
		RegisterMap: map[string][]string{
			"coil": {
				"00001,SystemEnableDisable,rw,Boolean",
				"00002,AlarmReset,wo,Boolean"},
			"discreteInput": {
				"10001,OverTemperatureAlarm,ro,Boolean",
				"10002,SensorFaultAlarm,ro,Boolean"},
			"holdingRegister": {
				"40000,SetpointTemperature,rw,16-bit INT",
				"40001,TemperatureCalibration,rw,16-bit INT",
				"40002,SamplingInterval,rw,16-bit INT"},
			"inputRegister": {
				"30001,AmbientTemperature,ro,16-bit INT",
				"30002,Sensor1Temperature,ro,16-bit INT",
				"30003,Sensor2Temperature,ro,16-bit INT"},
		},
		ServicesMap: components.Services{
			access.SubPath: &access,
		},
	}
	return uat
}

//-------------------------------------Instatiate the unit assets based on configuration

// newResource creates the Resource resource with its pointers and channels based on the configuration
func newResource(uac UnitAsset, sys *components.System, servs []components.Service) ([]components.UnitAsset, func()) {
	endpoint := uac.ServerAddress
	fmt.Printf("Trying to connect to server @ %s\n", endpoint)
	slave, err := net.Dial("tcp", endpoint)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connected")

	var slaveIO []components.UnitAsset
	for kind, gio := range uac.RegisterMap {
		ioKind := typeOfIO(kind)
		for _, str := range gio {
			newUA := &UnitAsset{} // Create a pointer to UnitAsset
			newUA.conn = &slave
			newUA.IOtype = ioKind
			parts := strings.Split(str, ",")
			if len(parts) < 4 {
				log.Fatalf("Bad configuration of %s\n", ioKind)
			}
			newUA.Address = parts[0]
			newUA.Name = parts[1]
			newUA.Access = parts[2]
			newUA.DataType = parts[3]
			newUA.Owner = sys
			newUA.Details = uac.Details
			newUA.ServicesMap = components.CloneServices(servs)
			slaveIO = append(slaveIO, newUA) // Use the pointer to newUA
		}
	}

	// Return the unit asset(s) and a cleanup function to close any connection
	return slaveIO, func() {
		fmt.Println("Closing the Modbus TCP connection")
		defer slave.Close()
	}
}

// -------------------------------------Unit asset's function methods

type ioType int

const (
	Coil ioType = iota
	DiscreteInput
	HoldingRegister
	InputRegister
	numberOfIOtype //to be use as arithmetic counter (IOType + 1) % numberOfIOtype
)

func typeOfIO(nameIO string) ioType {
	var ioMap = map[string]ioType{
		"coil":            Coil,
		"discreteInput":   DiscreteInput,
		"holdingRegister": HoldingRegister,
		"inputRegister":   InputRegister,
	}
	if io, ok := ioMap[nameIO]; ok {
		return io
	}
	log.Fatalf("error with IO type name")
	return -1 // an error has occured (which does not get executed)
}

// String returns the name of the type IO
func (iot ioType) String() string {
	dayNames := []string{"Coil", "DiscreteInput", "HoldingRegister", "InputRegister"}
	if iot < Coil || iot > InputRegister {
		return "Unknown"
	}
	return dayNames[iot]
}

func (ua *UnitAsset) read() (f forms.Form) {
	const unitID uint8 = 1 // Assuming a fixed Unit ID for simplification, which is not right
	address, err := strconv.ParseUint(ua.Address, 10, 16)
	if err != nil {
		log.Fatalf("Invalid address: %v", err)
	}

	// Initialize the request frame
	request := make([]byte, 12)
	binary.BigEndian.PutUint16(request[0:2], 1) // Transaction Identifier
	binary.BigEndian.PutUint16(request[2:4], 0) // Protocol Identifier
	binary.BigEndian.PutUint16(request[4:6], 6) // Length
	request[6] = unitID                         // Unit Identifier

	// Setting Function Code and Address based on IOtype
	switch ua.IOtype {
	case Coil:
		request[7] = 1 // Function Code for Read Coils
	case DiscreteInput:
		request[7] = 2 // Function Code for Read Discrete Inputs
	case HoldingRegister:
		request[7] = 3 // Function Code for Read Holding Registers
	case InputRegister:
		request[7] = 4 // Function Code for Read Input Registers
	default:
		log.Fatal("Unknown IO type")
	}

	binary.BigEndian.PutUint16(request[8:10], uint16(address)) // Adjusting for zero-based addressing
	binary.BigEndian.PutUint16(request[10:12], 1)              // Quantity to read: 1

	_, err = (*ua.conn).Write(request)
	if err != nil {
		log.Fatalf("Failed to send request: %v", err)
	}

	fmt.Printf("The request frame is: %+v\n", request)

	// Reading the response
	response := make([]byte, 256)
	n, err := (*ua.conn).Read(response)
	if err != nil || n < 9 {
		log.Fatalf("Failed to read response or response too short: %v", err)
	}

	fmt.Printf("The response frame is: %+v\n", response)

	// Parsing the response
	// The response structure is different for binary vs. analog values,
	// so we parse accordingly.
	if ua.IOtype == Coil || ua.IOtype == DiscreteInput {
		// For binary types, the response contains the status of coils/discrete inputs.
		// The status is in the byte immediately after the header.
		// Since we're reading a single coil/input, we only care about the first bit.
		coilStatus := response[9] & 0x01 // Getting the status of the first coil/input
		fmt.Println("Binary value:", coilStatus)
		coilStatusBool := coilStatus != 0
		var binaryForm forms.SignalB_v1a
		binaryForm.NewForm()
		binaryForm.Value = coilStatusBool
		binaryForm.Timestamp = time.Now()
		f = &binaryForm
	} else if ua.IOtype == HoldingRegister || ua.IOtype == InputRegister {
		// For analog types, the response contains the values of holding/input registers.
		if n < 11 { // Ensure enough bytes for a single register value
			log.Fatal("Incomplete response for register value")
		}
		registerValue := binary.BigEndian.Uint16(response[9:11])
		fmt.Println("Register value:", registerValue)
		var analogueForm forms.SignalA_v1a
		analogueForm.NewForm()
		analogueForm.Value = float64(registerValue)
		analogueForm.Unit = "undefined"
		analogueForm.Timestamp = time.Now()
		f = &analogueForm
	}
	return f
}
