# mbaigo System: thermostat

The idea of a thermostat is to control the temperature within an enclosure (e.g., the kitchen) by comparing the desired temperature and the actual temperature and then “actuating” a heater.
The thermostat system consumes the services from a temperature system and from a valve system.
It regulates the valve position (assuming a hydronic system) based on the current temperature and its set point.
The thermostat system consumes services from the Service Registrar and the Orchestrator.

It offers three services, *setpoint*, *thermalerror* and *jitter*. The setpoint can be read (e.g., GET) or set (e.g., PUT). The error signal is the difference between the setpoint or desired temperature and the current temperature. It can only be read. The jitter is the time it takes to obtain a new temperature reading and setting the new valve position.

The control loop is executed every 10 seconds, and can be configured.

## Compiling
To compile the code, one needs to get the AiGo module
```go get github.com/sdoque/mbaigo```
and initialize the *go.mod* file with ``` go mod init github.com/sdoque/systems/thermostat``` before running *go mod tidy*.

To run the code, one just needs to type in ```go run thermostat.go thing.go``` within a terminal or at a command prompt.

It is **important** to start the program from within its own directory (and each system should have their own directory) because it looks for its configuration file there. If it does not find it there, it will generate one and shutdown to allow the configuration file to be updated.

The configuration and operation of the system can be verified using the system's web server using a standard web browser, whose address is provided by the system at startup.

To build the software for one's own machine,
```go build -o thermostat_imac```, where the ending is used to clarify for which platform the code has been compiled for.


## Cross compiling/building
The following commands enable one to build for different platforms:
- Intel Mac:  ```GOOS=darwin GOARCH=amd64 go build -o thermostat_imac thermostat.go thing.go```
- ARM Mac: ```GOOS=darwin GOARCH=arm64 go build -o thermostat_amac thermostat.go thing.go```
- Windows 64: ```GOOS=windows GOARCH=amd64 go build -o thermostat.exe thermostat.go thing.go```
- Raspberry Pi 64: ```GOOS=linux GOARCH=arm64 go build -o thermostat_rpi64 thermostat.go thing.go```
- Linux: ```GOOS=linux GOARCH=amd64 go build -o thermostat_linux thermostat.go thing.go```

One can find a complete list of platform by typing *‌go tool dist list* at the command prompt

If one wants to secure copy it to a Raspberry pi,
`scp thermostat_rpi64 username@ipAddress:mbaigo/thermostat/` where user is the *username* @ the *IP address* of the Raspberry Pi with a relative (to the user's home directory) target *mbaigo/thermostat/* directory.