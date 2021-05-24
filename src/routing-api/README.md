# CF Routing API Server

The purpose of the Routing API is to present a RESTful interface for registering
and deregistering routes for both internal and external clients. This allows
easier consumption by different clients as well as the ability to register
routes from outside of the CF deployment.

**Note**: This repository should be imported as `code.cloudfoundry.org/routing-release/routing-api`.

## Reporting issues and requesting features

Please report all issues and feature requests in [cloudfoundry/routing-release](https://github.com/cloudfoundry/routing-release).

## Downloading and Installing

### External Dependencies

- Go should be installed and in the PATH
- This repo is part of
  [routing-release](https://github.com/cloudfoundry/routing-release) bosh
  release repo, which also acts as cannonical GOPATH. So to work on routing-api
  you will need to checkout
  [routing-release](https://github.com/cloudfoundry/routing-release) and follow
  instructions in its
  [README](https://github.com/cloudfoundry/routing-release/blob/develop/README.md#developer-workflow)
  to setup GOPATH.


### Development Setup

Refer to routing-release
[README](https://github.com/cloudfoundry/routing-release/blob/develop/README.md#developer-workflow)
for development setup.

## Development

To run the tests you need a running RDB (either Postgres or MySQL). Currently
there is a helper script under routing-release which runs tests in [docker
container](https://github.com/cloudfoundry/routing-release/blob/develop/scripts/run-unit-tests-in-docker).
`cf-routing-pipeline` docker image used in the below script is configured with
correct version of `MySQL` and `Postgres` for testing purposes. To run the tests
for routing-api:

```sh
./scripts/run-unit-tests-in-docker routing-api
```

### Local Database Instances in Docker
A Docker Compose is provided to start the database locally for testing.
```console
$ docker-compose up
...
postgress_1  | 2020-11-12 18:00:21.618 UTC [1] LOG:  database system is ready to accept connections
mysql_1      | [Entrypoint] Starting MySQL 5.7.32-1.1.18
```

## Running the API Server

### Server Configuration

#### jwt token

To run the routing-api server, a configuration file with the public uaa jwt token must be provided.
This configuration file can then be passed in with the flag `-config [path_to_config]`.
An example of the configuration file can be found under `example_config/example.yml` for bosh-lite.

To generate your own config file, you must provide a `uaa_verification_key` in
pem format, such as the following:

```yaml
uaa_verification_key: |
  -----BEGIN PUBLIC KEY-----
  SOME_KEY
  -----END PUBLIC KEY-----
```

This can be found in your Cloud Foundry manifest under `uaa.jwt.verification_key`

#### Oauth Clients

The Routing API uses OAuth tokens to authenticate clients. To obtain a token
from UAA that grants the API client permission to register routes, an OAuth
client must first be created for the API client in UAA. An API client can then
authenticate with UAA using the registered OAuth client credentials, request a
token, then provide this token with requests to the Routing API.

Registering OAuth clients can be done using the cf-release BOSH deployment
manifest, or manually using the `uaac` CLI for UAA.

- For API clients that wish to register/unregister routes with the Routing API,
  the OAuth client in UAA must be configured with the `routing.routes.write`
  authority.
- For API clients that wish to list routes with the Routing API, the OAuth
  client in UAA must be configured with the `routing.routes.read` authority.
- For API clients that wish to list router groups with the Routing API, the
  OAuth client in UAA must be configured with the `routing.router_groups.read`
  authority.

For instructions on fetching a token, please refer to the [API
documentation](docs/api_docs.md).

##### Configure OAuth clients in the cf-release BOSH Manifest

```yaml
uaa:
  clients:
    routing_api_client:
      authorities: routing.routes.write,routing.routes.read,routing.router_groups.read
      authorized_grant_type: client_credentials
      secret: route_secret
```

##### Configure OAuth clients manually using `uaac` CLI for UAA

1. Install the `uaac` CLI

   ```bash
   gem install cf-uaac
   ```

2. Get the admin client token

   ```bash
   uaac target uaa.bosh-lite.com
   uaac token client get admin # You will need to provide the client_secret, found in your CF manifest.
   ```

3. Create the OAuth client.

   ```bash
   uaac client add routing_api_client \
     --authorities "routing.routes.write,routing.routes.read,routing.router_groups.read" \
     --authorized_grant_type "client_credentials"
   ```

### Starting the Server

To run the API server you need to provide RDB configuration for the Postgres or
MySQL, a configuration file containing the public UAA jwt key, plus some
optional flags.

```bash
routing-api \
   -ip 127.0.0.1 \
   -systemDomain 127.0.0.1.xip.io \
   -config example_config/example.yml \
   -port 3000 \
   -maxTTL 60
```


### Profiling the Server

The Routing API runs the
[cf_debug_server](https://github.com/cloudfoundry/debugserver), which is a
wrapper around the go pprof tool. In order to generate this profile, do the
following:

```bash
# Establish a SSH tunnel to your server (not necessary if you can connect directly)
ssh -L localhost:8080:[INTERNAL_SERVER_IP]:17002 vcap@[BOSH_DIRECTOR]
# Run the profile tool.
go tool pprof http://localhost:8080/debug/pprof/profile
```

> Note: Debug server should run on loopback interface i.e., 0.0.0.0 for the SSH
> tunnel to work.

## Using the API

The Routing API uses OAuth tokens to authenticate clients. To obtain a token
from UAA an OAuth client must first be created for the API client in UAA. For
instructions on registering OAuth clients, see [Server
Configuration](#oauth-clients).

### Using the API with the `rtr` CLI

A CLI client called `rtr` has been created for the Routing API that simplifies
interactions by abstracting authentication.

- [Documentation](https://github.com/cloudfoundry/routing-api-cli)
- [Releases](https://github.com/cloudfoundry/routing-api-cli/releases)

### Using the API manually

Please refer to the [API documentation](docs/api_docs.md).

## Known issues

+ The routing-api will return a 404 if you attempt to hit the endpoint
  `http://[router host]/routing/v1/routes/` as opposed to `http://[router
  host]/routing/v1/routes`
