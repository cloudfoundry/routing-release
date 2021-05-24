route-registrar
===============

A standalone executable written in golang that continuously broadcasts a routes
to the [gorouter](https://github.com/cloudfoundry/gorouter).  This is designed
to be a general purpose solution, packaged as a BOSH job to be colocated with
components that need to broadcast their routes to the gorouter, so that those
components don't need to maintain logic for route registration.

> **Note**: This repository should be imported as `code.cloudfoundry.org/routing-release/route-registrar`.

* CI: [Concourse](https://networking.ci.cf-app.com/teams/ga/pipelines/routing)

## Reporting issues and requesting features

Please report all issues and feature requests in [cloudfoundry/routing-release](https://github.com/cloudfoundry/routing-release).

## Usage

1. Clone the [routing-release repository](https://github.com/cloudfoundry/routing-release)
  ```bash
  git clone --recursive https://github.com/cloudfoundry/routing-release
  ```

1. Build the route-registrar binary
  ```bash
  cd routing-release
  source .envrc # or direnv allow
  go get code.cloudfoundry.org/routing-release/route-registrar
  ls -la bin/route-registrar
  ```

1. The route-registrar expects a configuration json file like the one below:
  ```json
{
  "message_bus_servers": [
    {
      "host": "NATS_SERVER_HOST:PORT",
      "user": "NATS_SERVER_USERNAME",
      "password": "NATS_SERVER_PASSWORD"
    }
  ],
  "host": "HOSTNAME_OR_IP_OF_ROUTE_DESTINATION",
  "routes": [
    {
      "name": "SOME_ROUTE_NAME",
      "tls_port": "TLS_PORT_OF_ROUTE_DESTINATION",
      "tags": {
        "optional_tag_field": "some_tag_value",
        "another_tag_field": "some_other_value"
      },
      "uris": [
        "some_source_uri_for_the_router_to_map_to_the_destination",
        "some_other_source_uri_for_the_router_to_map_to_the_destination"
      ],
      "server_cert_domain_san": "some.service.internal",
      "route_service_url": "https://route-service.example.com",
      "registration_interval": "REGISTRATION_INTERVAL",
      "health_check": {
        "name": "HEALTH_CHECK_NAME",
        "script_path": "/path/to/check/executable",
        "timeout": "HEALTH_CHECK_TIMEOUT"
      }
    }
  ]
}
  ```
  - `message_bus_servers` is an array of data with location and credentials for
    the NATS servers; route-registrar currently registers and deregisters routes
    via NATS messages. `message_bus_servers.host` must include both hostname and
    port; e.g. `host: 10.0.32.11:4222`
  - `host` is the destination hostname or IP for the routes being registered. To
    Gorouter, these are backends.
  - `routes` is required and is an array of hashes. For each route collection:
    - `name` must be provided and be a string
    - `port` or `tls_port` are for the destination host (backend). At least one
      must be provided and must be a positive integer > 1.
    - `server_cert_domain_san` is the SAN on the destination host's TLS
      certificate. Required when `tls_port` is provided.
    - `uris` are the routes being registered for the destination `host`. Must be
      provided and be a non empty array of strings.  All URIs in a given route
      collection will be mapped to the same host and port.
    - `registration_interval` is the interval for which routes are registered
      with NATS. Must be provided and be a string with units (e.g. "20s"). It
      must parse to a positive time duration e.g. "-5s" is not permitted.
    - `route_service_url` is optional. When provided, Gorouter will proxy
      requests received for the `uris` above to this address.
    - `health_check` is optional and explained in more detail below.

1. Run route-registrar binaries using the following command
  ```bash
  route-registrar -configPath FILE_PATH_TO_CONFIG_JSON -pidfile PATH_TO_PIDFILE
  ```

### SNI Routing
The route registrar can be used to setup SNI routing. This is an example route json:
```
{
  "routes": [
    {
      "type": "sni",
      "external_port": "TLS_PORT_OF_ROUTE_SOURCE",
      "name": "SOME_ROUTE_NAME",
      "sni_port": "TLS_PORT_OF_ROUTE_DESTINATION",
    }
  ]
}
```
### Health check

If the `health_check` is not configured for a route collection, the routes are continually registered according to the `registration_interval`.

If the `health_check` is configured, the executable provided at
`health_check.script_path` is invoked and the following applies:
- if the executable exits with success, the routes are registered.
- if the executable exits with error, the routes are deregistered.
- if `health_check.timeout` is configured, it must parse to a positive time
  duration (similar to `registration_interval`), and the executable must exit
  within the timeout. If the executable does not terminate within the timeout,
  it is forcibly terminated (with `SIGKILL`) and the routes are deregistered.
- if `health_check.timeout` is not configured, the executable must exit within
  half the `registration_interval`. If the executable does not terminate within
  the timeout, it is forcibly terminated (with `SIGKILL`) and the routes are
  deregistered.

## BOSH release

This program is packaged as a
[job](https://github.com/cloudfoundry/routing-release/tree/master/jobs/route_registrar)
and a
[package](https://github.com/cloudfoundry/routing-release/tree/master/packages/route_registrar)
in the [routing-release](https://github.com/cloudfoundry/routing-release) BOSH
release, it can be colocated with the following manifest changes:

```yaml
releases:
- name: routing
- name: my-release

jobs:
- name: myJob
  templates:
  - name: my-job
    release: my-release
  - name: route_registrar
    release: routing
  properties:
    route_registrar:
      # ...
      # [ see bosh job spec ]
```

## Development

### Dependencies

Dependencies are saved using the
[routing-release](https://github.com/cloudfoundry/routing-release). Just clone
the repo and set the `GOPATH` to the routing release directory.

### Running tests

1. Install the ginkgo binary with `go get`:
  ```bash
  go get github.com/onsi/ginkgo/ginkgo
  ```

1. Run tests, by running the following command from root of this repository
  ```bash
  bin/test
  ```
