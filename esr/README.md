# Ephemeral Service Registry System

The Ephemeral Service Registry (esr) system is an alternative service registrar. It does not use an SQL database but only a simple map of unique ID number associated with a service record.
The service registrar is one of the mandatory core system of an Arrowhead local cloud.
It keeps track of the currently available services within that cloud.

The ephemeral or non persistent aspect of the registry reflects that only the current available service records are kept.
There is no need to permanently keep track of what is currently available.
If such tracking is necessary, it is best suited with the Modeler system with its graph database as asset.

## Cross compile
- Intel Mac: ```GOOS=darwin GOARCH=amd64 go build -o esr_imac esr.go thing.go scheduler.go``` 
- ARM Mac: ```GOOS=darwin GOARCH=arm64 go build -o esr_amac esr.go thing.go scheduler.go```
- Windows 64: ```GOOS=windows GOARCH=amd64 go build -o esr_win64.exe esr.go thing.go scheduler.go```
- Raspberry Pi 64: ```GOOS=linux GOARCH=arm64 go build -o esr_rpi64 esr.go thing.go scheduler.go```
- Linux: ```GOOS=linux GOARCH=amd64 go build -o esr_amd64 esr.go thing.go scheduler.go```

## Testing shutdown
To test the graceful shutdown, one cannot use the IDE debugger but must use the terminal with
```go run esr.go thing.go scheduler.go```
Using the IDE debugger will allow one to test device failure, i.e. unplugging the computer.