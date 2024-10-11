# mbaigo System: parallax

The Parallax system is an actuator demonstrator. It is a service provider. It uses a [Parallax servo motor](https://www.parallax.com/package/parallax-standard-servo-downloads/) with a PWM signal. 

The system offers one service per servomotor, *rotation*. It can be read or set (e.g., GET or PUT). The values are in percent of full range.

## Asset deployment 
	
Connect the data line to GPIO 18 as in [this example](https://randomnerdtutorials.com/raspberry-pi-pwm-python/).

## Compiling
To compile the code, one needs to get the mbaigo module
```go get github.com/sdoque/mbaigo```
and initialize the *go.mod* file with ``` go mod init github.com/sdoque/systems/parallax``` before running *go mod tidy*.

To run the code, one just needs to type in ```go run parallax.go thing.go``` within a terminal or at a command prompt.

It is **important** to start the program from within it own directory (and each system should have their own directory) because it looks for its configuration file there. If it does not find it there, it will generate one and shutdown to allow the configuration file to be updated.

The configuration and operation of the system can be verified using the system's web server using a standard web browser, whose address is provided by the system at startup.

To build the software for one's own machine,
```go build -o parallax_imac```, where the ending is used to clarify for which platform the code is for.


## Cross compiling/building
The following commands enable one to build for different platforms:
- Raspberry Pi 64: ```GOOS=linux GOARCH=arm64 go build -o parallax_rpi64 parallax.go thing.go```

One can find a complete list of platform by typing *‌go tool dist list* at the command prompt

If one wants to secure copy it to a Raspberry pi,
`scp parallax_rpi64 username@ipAddress:mbaigo/parallax/` where user is the *username* @ the *ipAddress* of the Raspberry Pi with a relative (to the user's home directory) target *mbaigo/parallax/* directory.