# mbaigo System: Telegrapher

The Telegrapher system has for asset an MQTT broker such that it can offer as a service the broker’s services, which can published or subscribed to.

## Compiling
To compile the code, one needs to get the mbaigo module
```go get github.com/sdoque/mbaigo```
and initialize the *go.mod* file with ``` go mod init github.com/sdoque/arrowsys/telegrapher``` before running *go mod tidy*.

The reason the *go.mod* file is not included in the repository is that when developing the mbaigo module, a replace statement needs to be included to point to the development code.

To run the code, one just needs to type in ```go run telegrapher.go thing.go``` within a terminal or at a command prompt.

It is **important** to start the program from within it own directory (and each system should have their own directory) because it looks for its configuration file there. If it does not find it there, it will generate one and shutdown to allow the configuration file to be updated.

The configuration and operation of the system can be verified using the system's web server using a standard web browser, whose address is provided by the system at startup.

To build the software for one's own machine,
```go build -o telegrapher_imac```, where the ending is used to clarify for which platform the code is for.


## Cross compiling/building
The following commands enable one to build for different platforms:
- Raspberry Pi 64: ```GOOS=linux GOARCH=arm64 go build -o telegrapher_rpi64 telegrapher.go thing.go```
One can find a complete list of platform by typing *‌go tool dist list* at the command prompt

If one wants to secure copy it to a Raspberry pi,
`scp telegrapher_rpi64 jan@192.168.1.195:demo/telegrapher/` where user is the *username* @ the *IP address* of the Raspberry Pi with a relative (to the user's home directory) target *demo/telegrapher/* directory.telegrapher

## Deploying the asset
On a Raspberry Pi, typing one line at the time,

```
sudo apt update && sudo apt upgrade
sudo apt install -y mosquitto mosquitto-clients
mosquitto -v
```

To publish to a topic on the host type ```mosquitto _pub -h localhost -t /test/topic -m "Hello from localhost"```  and to subscribe to a topic ```mosquitto _sub -h localhost -t /test/topic```

### Adding some security
Add the following by editing the configuration file ```sudo nano /etc/mosquitto/mosquitto.conf```

```
listener 1883
allow_anonymous false
password_file /etc/mosquitto/pwdfile
```


Adding a user with a password prompt ```sudo mosquitto_passwd -c /etc/mosquitto/pwdfile publisher_user``` and then adding with password in the command ```sudo mosquitto_passwd -b /etc/mosquitto/pwdfile subscriber_user subpwd```

The broker has to be restarted ```sudo service mosquitto restart```

Example command line statement 
  ```mosquitto_sub -h localhost -t kitchen/temperature -u user -P password```
