# envoy-tcp-router

Envoy-based TCP router for Cloud Foundry. This repository is intended to be consumed as a git submodule of [routing-release](https://github.com/cloudfoundry/routing-release) at `src/code.cloudfoundry.org/envoy-tcp-router`.

## Components

- **`main.go`** — xDS control plane entry point. Polls the CF Routing API and pushes Listener/Cluster snapshots to Envoy via gRPC.
- **`controlplane/`** — gRPC xDS server (`server.go`) and CF route-to-Envoy translator (`translator.go`).
- **`filter/`** — L4 Go network filter loaded by Envoy as a shared library (`filter.so`). Logs identity on new TCP connections and always allows (phase 1).

## How it works

The control plane translates `TcpRouteMapping` objects from the CF Routing API into Envoy Listener + Cluster resources. Multiple apps on the same external port are distinguished by SNI hostname via `FilterChainMatch.ServerNames`.

## Building

This repo has no standalone `go.mod`. It is built inside the routing-release Go workspace at `src/code.cloudfoundry.org/`:

```bash
# Control plane binary
go build -o envoy-tcp-router code.cloudfoundry.org/envoy-tcp-router

# L4 Go filter shared library
go build -buildmode=c-shared -o filter.so code.cloudfoundry.org/envoy-tcp-router/filter
```
