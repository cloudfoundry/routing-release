---
title: Usage
expires_at: never
tags: [routing-release,routing-api-cli]
---

Each command has required arguments and route structure.

Required arguments:

**--api**: the routing API endpoint, e.g. `http://api.10.244.0.34.xip.io`<br />
**--client-id**: the id of the client registered with your OAuth provider with the [proper authorities](https://github.com/cloudfoundry/routing-api#oauth-clients), e.g. `routing_api_client`<br />
**--client-secret**: your OAuth client secret, e.g. `route_secret`<br />
**--oauth-url**: the OAuth provider endpoint with optional port, e.g. `http://uaa.10.244.0.34.xip.io`

Optional arguments:
**--skip-tls-verification**: Skip TLS verification when talking to UAA and Routing API.

Routes are described as JSON: `'[{"route":"foo.com","port":65340,"ip":"1.2.3.4","ttl":60, "route_service_url":"https://route-service.example.cf-app.com"}]'`

### List Routes
```bash
rtr list [args]
```

### Register Route(s)
```bash
rtr register [args] [routes]
```

### Unregister Route(s)
```bash
rtr unregister [args] [routes]
```
### Subscribe to Events
```bash
rtr events [args]
```

### Tracing Requests and Responses

By specifying the environment variable `RTR_TRACE=true`, `rtr` will output the HTTP requests and responses that it makes and receives.
```bash
export RTR_TRACE=true
rtr list [args]
```

Notes:
- Route "ttl" definition is ignored for unregister.
- CLI will appear successful when unregistering routes that do not exist.
- The `route_service_url` is an optional value, and must be a HTTPS url.

###Examples

```bash
rtr list --api https://api.example.com --client-id admin --client-secret admin-secret --oauth-url https://uaa.example.com

rtr register --api https://api.example.com --client-id admin --client-secret admin-secret --oauth-url https://uaa.example.com '[{"route":"mynewroute.com","port":12345,"ip":"1.2.3.4","ttl":60}]'

rtr unregister --api https://api.example.com --client-id admin --client-secret admin-secret --oauth-url https://uaa.example.com '[{"route":"undesiredroute.com","port":12345,"ip":"1.2.3.4"}]'
```
