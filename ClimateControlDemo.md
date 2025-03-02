# My Project

Welcome to **My Project**! This repository demonstrates how to use Mermaid diagrams in a GitHub `README.md` file.

## Features
- Service registration process
- Service provider discovery
- Periodic temperature updates

---

## Sequence Diagram: Service Registration

This diagram shows how services register themselves with the `ServiceRegistrar`.

```mermaid
sequenceDiagram
    participant ServiceRegistrar
    participant Thermostat
    participant Orchestrator
    participant ds18b20
    participant Parallax

    loop Before Registration Expiration
        ServiceRegistrar->>ServiceRegistrar: Register Services
        ServiceRegistrar-->>ServiceRegistrar: Registration Expiration Time

        Thermostat->>ServiceRegistrar: Register Services
        ServiceRegistrar-->>Thermostat: Registration Expiration Time

        Orchestrator->>ServiceRegistrar: Register Services
        ServiceRegistrar-->>Orchestrator: Registration Expiration Time

        ds18b20->>ServiceRegistrar: Register Services
        ServiceRegistrar-->>ds18b20: Registration Expiration Time

        Parallax->>ServiceRegistrar: Register Services
        ServiceRegistrar-->>Parallax: Registration Expiration Time
    end
```


```mermaid
graph TD
    actor_Thermostat[Actor: Thermostat]
    actor_Orchestrator[Actor: Orchestrator]
    actor_ServiceRegistrar[Actor: Service Registrar]
    actor_ds18b20[Actor: ds18b20]
    actor_Parallax[Actor: Parallax]

    subgraph UseCases
        UC_Register[Register Services]
        UC_Discover[Discover Service Provider]
        UC_GetTemp[Get Current Temperature]
        UC_UpdateValve[Update Valve Position]
        UC_CheckServices[Check for Services]
        UC_ProvideURL[Provide Service URL]
    end

    actor_Thermostat --> UC_Register
    actor_Thermostat --> UC_Discover
    actor_Thermostat --> UC_GetTemp
    actor_Thermostat --> UC_UpdateValve

    actor_Orchestrator --> UC_Discover
    actor_Orchestrator --> UC_CheckServices

    actor_ServiceRegistrar --> UC_Register
    actor_ServiceRegistrar --> UC_ProvideURL

    actor_ds18b20 --> UC_Register
    actor_ds18b20 --> UC_GetTemp

    actor_Parallax --> UC_Register
    actor_Parallax --> UC_UpdateValve

```

```mermaid
stateDiagram-v2
    [*] --> CheckTemperature
    CheckTemperature --> CalculateValvePosition
    CalculateValvePosition --> UpdateValve
    UpdateValve --> [*]
```

```mermaid
%% Mermaid Activity Diagram for main()
flowchart TD
    Start([Start]) --> PrepareShutdown[Prepare for Graceful Shutdown]
    PrepareShutdown -->|Create Cancelable Context| InstantiateSystem[Instantiate the System]
    InstantiateSystem -->|Set Husk Details| SetupHusk[Set up Husk Details]
    SetupHusk -->|Instantiate Template Unit Asset| InitTemplate[Initialize Template Asset]
    InitTemplate -->|Configure the System| ConfigureSystem[Configure System]
    ConfigureSystem -->|Handle Configuration Errors| ErrorCheck{Error?}
    ErrorCheck -- Yes --> FatalError[Log Fatal Error and Exit]
    ErrorCheck -- No --> ParseResources[Parse Raw Resources]

    ParseResources -->|Generate Unit Assets| GenerateUAssets[Generate Unit Assets]
    GenerateUAssets -->|Generate PKI Keys and CSR| GenerateKeys[Request Certificate from CA]
    GenerateKeys -->|Register System and Services| RegisterServices[Register Services]
    RegisterServices -->|Start Request Handlers| StartServers[Start Request Handlers and Servers]
    StartServers -->|Wait for SIGINT Signal| WaitSignal[Wait for Shutdown Signal]
    WaitSignal --> ShutdownSignal{Shutdown Signal?}
    ShutdownSignal -- Yes --> GracefulShutdown[Shutdown System and Goroutines]
    GracefulShutdown --> End([End])
    ShutdownSignal -- No --> WaitSignal
```