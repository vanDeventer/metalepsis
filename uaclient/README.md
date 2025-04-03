# mbaigo System: UAclient

## Purpose
This system offers as services specific nodes from an OPC UA server. It also has the browsing service that enables the deployment technician to see what nodes are available from the server.

The herewith Arrowhead framework system must be configured at deployment time for each OPC UA node. So to add node with id "ns=3;i=1002”, one must add it to the list of nodeID. (The Prosys OPC [simulation server](https://prosysopc.com/products/opc-ua-simulation-server/) was used here with "ns=3;i=1002" being the counter node that increases by one every second.)

```json
 "unit_assets": [
      {
         "name": "PLC with OPC UA server",
         "details": {
            "KKS": [
               "YLLCP001"
            ],
            "Location": [
               "Line 1"
            ],
            "PLC": [
               "Prosys OPC UA Simulation Server"
            ]
         },
         "serverAddress": "opc.tcp://192.168.1.2:53530/OPCUA/SimulationServer",
         "NodeList": {
            "Node_Id": [
                "ns=3;i=1002"
            ]
},
         "Server": null,
         "NodeID": null,
         "NodeClass": 0,
         "NodeName": "",
         "BrowseName": "",
         "Description": "",
         "AccessLevel": 0,
         "Path": "",
         "DataType": "",
         "Writable": false,
         "Unit": "",
         "Scale": "",
         "Min": "",
         "Max": ""
      }
   ],

```

A unit block needs to be added for each node. A comma separates the resource blocks.

**Status:**
- read only (write to come)
- Node ID only (type and name to come)
- No server-client certificate yet
- No subscriptions yet

## Compiling
To compile the code, one needs to get the AiGo module
```go get github.com/sdoque/aigo```
and initialize the *go.mod* file with ``` go mod init github.com/sdoque/systems/uaclient``` before running *go mod tidy*.

To run the code, one just needs to type in ```go run uaclient.go thing.go``` within a terminal or at a command prompt.

It is **important** to start the program from within its own directory (and each system should have their own directory) because it looks for its configuration file there. If it does not find it there, it will generate one and shutdown to allow the configuration file to be updated.

The configuration and operation of the system can be verified using the system's web server using a standard web browser, whose address is provided by the system at startup.

To build the software for one's own machine,
```go build -o uaclient_imac```, where the ending is used to clarify for which platform the code is for.


## Cross compiling/building
The following commands enable one to build for different platforms:

- Raspberry Pi 64: ```GOOS=linux GOARCH=arm64 go build -o uaclient_rpi64 uaclient.go thing.go```

One can find a complete list of platform by typing *‌go tool dist list* at the command prompt

If one wants to secure copy it to a Raspberry pi,
`scp uaclient_rpi64 username@ipAddress:Desktop/opcuac/` where user is the *username* @ the *IP address* of the Raspberry Pi with a relative (to the user's home directory) target *mbaigo/uaclient/* directory.