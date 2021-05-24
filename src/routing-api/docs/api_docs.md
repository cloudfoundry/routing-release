Routing API Documentation
=========================

Reference documentation for client authors using the Routing API manually.

### Authorization Token

To obtain an token from UAA, use the `uaac` CLI for UAA.

1. Install the `uaac` CLI:

   ```bash
   gem install cf-uaac
   ```

1. Set the UAA target:

   ```bash
   uaac target uaa.example.com
   ```

1. Retrieve the OAuth token using credentials for registered OAuth client:

   ```bash
   uaac token client get routing_api_client
   ```

1. Display the `access_token`, which can be used as the Authorization header to
   `curl` the Routing API:

   ```bash
   uaac context
   ```

Routing API Endpoints
---------------------
- [Create Router Groups](#create-router-groups)
- [Delete Router Groups](#delete-router-groups)
- [List Router Groups](#list-router-groups)
- [Update Router Group](#update-router-group)
- [List TCP Routes](#list-tcp-routes)
- [Create TCP Routes](#create-tcp-routes)
- [Delete TCP Routes](#delete-tcp-routes)
- [Subscribe to Events for TCP Routes](#subscribe-to-events-for-tcp-routes)
- [List HTTP Routes (Experimental)](#list-http-routes-experimental)
- [Create HTTP Routes (Experimental)](#create-http-routes-experimentalcreate)
- [Delete HTTP Routes (Experimental)](#delete-http-routes-experimental)
- [Subscribe to Events for HTTP Routes (Experimental)](#subscribe-to-events-for-http-routes-experimental)

Create Router Groups
---------------------
### Request
  `POST /routing/v1/router_groups`

#### Request Headers
  A bearer token for an OAuth client with `routing.router_groups.write` scope is
  required.

#### Request Body
  A JSON-encoded object for the modified router group. The `name` and `type`
  fields must be included, and `reservable_ports` must be included if `type` is
  tcp.

| Object Field       | Type   | Required? | Description |
|--------------------|--------|-----------|-------------|
| `name`             | string | yes       | Name of the router group.
| `type`             | string | yes       | Type of the router group e.g. `http` or `tcp`.
| `reservable_ports` | string | yes       | Comma delimited list of reservable port or port ranges. These ports must fall between 1024 and 65535 (inclusive).

#### Example Request
```bash
curl -vvv -H "Authorization: bearer [uaa token]" http://api.system-domain.com/routing/v1/router_groups -X POST -d '{"name": "my-router-group", "type": "http"}'
```
### Response
  Expected Status `201 Created`

#### Response Body
  A JSON-encoded object for the updated `Router Group`.

| Object Field       | Type   | Description |
|--------------------|--------|-------------|
| `guid`             | string | GUID of the router group.
| `name`             | string | External facing port for the TCP route.
| `type`             | string | Type of the router group e.g. `tcp`.
| `reservable_ports` | string | Comma delimited list of reservable port or port ranges. (For `type` of `TCP`)

#### Example Response:
```json
{
  "guid": "568c0232-e7c0-47ff-4c8a-bc89b49ade5b",
  "name": "my-router-group",
  "type": "http"
  "reservable_ports": ""
}
```

Delete Router Groups
---------------------
### Request
  `DELETE /routing/v1/router_groups/:guid`

#### Request Headers
  A bearer token for an OAuth client with `routing.router_groups.write` scope is required.

#### Example Request
```bash
curl -vvv -H "Authorization: bearer [uaa token]" http://api.system-domain.com/routing/v1/router_groups/:guid -X DELETE'
```
### Response
  Expected Status `204 No Content`

List Router Groups
-------------------
### Request
  `GET /routing/v1/router_groups`

#### Request Headers
  A bearer token for an OAuth client with `routing.router_groups.read` scope is required.

#### Request Parameters (Optional)

| Parameter       | Type   | Description |
|-----------------|--------|-------------|
| `name`          | string | Name of the router group |

#### Example request
```bash
curl -vvv -H "Authorization: bearer [uaa token]" http://api.system-domain.com/routing/v1/router_groups?name=default-tcp
```

### Response
  Expected Status `200 OK`

#### Response Body
  A JSON-encoded array of `Router Group` objects.

| Object Field       | Type   | Description |
|--------------------|--------|-------------|
| `guid`             | string | GUID of the router group.
| `name`             | string | External facing port for the TCP route.
| `type`             | string | Type of the router group e.g. `tcp`.
| `reservable_ports` | string | Comma delimited list of reservable port or port ranges.

#### Example Response
```json
[{
  "guid": "abc123",
  "name": "default-tcp",
  "reservable_ports":"1024-65535",
  "type": "tcp"
}]
```

Update Router Group
-------------------
To update a Router Group's `reservable_ports` field with a new port range.

### Request
  `PUT /routing/v1/router_groups/:guid`

  `:guid` is the GUID of the router group to be updated.

#### Request Headers
  A bearer token for an OAuth client with `routing.router_groups.write` scope is required.

#### Request Body
  A JSON-encoded object for the modified router group. Only the `reservable_ports` field may be updated.

| Object Field       | Type   | Required? | Description |
|--------------------|--------|-----------|-------------|
| `reservable_ports` | string | yes       | Comma delimited list of reservable port or port ranges. These ports must fall between 1024 and 65535 (inclusive).

  > **Warning:** If routes are registered for ports that are not in the new range,
  > modifying your load balancer to remove these ports will result in backends for
  > those routes becoming inaccessible.

#### Example Request
```bash
curl -vvv -H "Authorization: bearer [uaa token]" http://api.system-domain.com/routing/v1/router_groups/abc123 -X PUT -d '{"reservable_ports":"9000-10000"}'
```
### Response
  Expected Status `200 OK`

#### Response Body
  A JSON-encoded object for the updated `Router Group`.

| Object Field       | Type   | Description |
|--------------------|--------|-------------|
| `guid`             | string | GUID of the router group.
| `name`             | string | External facing port for the TCP route.
| `type`             | string | Type of the router group e.g. `tcp`.
| `reservable_ports` | string | Comma delimited list of reservable port or port ranges.

#### Example Response:
```json
{
  "guid": "abc123",
  "name": "default-tcp",
  "reservable_ports":"9000-10000",
  "type": "tcp"
}
```

List TCP Routes
-------------------
### Request
  `GET /routing/v1/tcp_routes`

#### Request Headers
  A bearer token for an OAuth client with `routing.routes.read` scope is required.

#### Request Parameters (Optional)
| Parameter           | Type   | Description |
|---------------------|--------|-------------|
| `isolation_segment` | string | Name of the isolation segment. If this parameter is included but a value is not given, then  tcp routes registered without a specified isolation segment will be returned. |

#### Example Requests
```bash
# returns all tcp routes
curl -vvv -H "Authorization: bearer [uaa token]" http://api.system-domain.com/routing/v1/tcp_routes

# filter for routes from multiple isolation segments
curl -vvv -H "Authorization: bearer [uaa token]" http://api.system-domain.com/routing/v1/tcp_routes?isolation_segment=is1&isolation_segment=is2

# filter for routes without a specified isolation segment
curl -vvv -H "Authorization: bearer [uaa token]" http://api.system-domain.com/routing/v1/tcp_routes?isolation_segment=

# filter for routes from multiple isolation segments and from the unspecified isolation segment
curl -vvv -H "Authorization: bearer [uaa token]" http://api.system-domain.com/routing/v1/tcp_routes?isolation_segment=&isolation_segment=is1&isolation_segment=is2
```

### Response
  Expected Status `200 OK`

#### Response Body
  A JSON-encoded array of `TCP Route` objects.

| Object Field        | Type            | Description |
|---------------------|-----------------|-------------|
| `router_group_guid` | string          | GUID of the router group associated with this route.
| `backend_port`      | integer         | Backend port. Must be greater than 0.
| `backend_ip`        | string          | IP address of backend.
| `port`              | integer         | External facing port for the TCP route.
| `modification_tag`  | object     | See [Modification Tags](modification_tags.md).
| `ttl`               | integer         | Time to live, in seconds. The mapping of backend to route will be pruned after this time.
| `isolation_segment` | string | Isolation segment for the route. |

#### Example Response:
```json
[{
  "router_group_guid": "xyz789",
  "backend_port": 60000,
  "backend_ip": "10.1.1.12",
  "port": 5200,
  "modification_tag":  {
    "guid": "cbdhb4e3-141d-4259-b0ac-99140e8998l0",
    "index": 10
  },
  "ttl": 120,
  "isolation_segment": ""
}]
```

Create TCP Routes
-------------------
As routes have a TTL, clients must register routes periodically to keep them active.

### Request
  `POST /routing/v1/tcp_routes/create`

#### Request Headers
  A bearer token for an OAuth client with `routing.routes.write` scope is required.

#### Request Body
  A JSON-encoded array of `TCP Route` objects for each route to register.

| Object Field        | Type            | Required? | Description |
|------------------------|-----------------|-----------|-------------|
| `router_group_guid`    | string          | yes       | GUID of the router group associated with this route.
| `port`                 | integer         | yes       | External facing port for the TCP route.
| `backend_ip`           | string          | yes       | IP address of backend
| `backend_port`         | integer         | yes       | Backend port. Must be greater than 0.
| `ttl`                  | integer         | yes       | Time to live, in seconds. The mapping of backend to route will be pruned after this time. Must be greater than 0 seconds and less than the configured value for max_ttl (default 120 seconds).
| `modification_tag`     | object          | no        | See [Modification Tags](modification_tags.md).
| `isolation_segment`    | string          | no        | Name of the isolation segment for the route.
| `backend_sni_hostname` | string          | no        | Sni backend hostname used for SNI routing. 

#### Example Request
```bash
curl -vvv -H "Authorization: bearer [uaa token]" -X POST http://api.system-domain.com/routing/v1/tcp_routes/create -d '
[{
  "router_group_guid": "xyz789",
  "port": 5200,
  "backend_ip": "10.1.1.12",
  "backend_port": 60000,
  "ttl": 120,
  "modification_tag":  {
    "guid": "cbdhb4e3-141d-4259-b0ac-99140e8998l0",
    "index": 1
  }
}]'
```
#### Example Request with SNI
```bash
curl -k -vvv -H "Authorization: bearer $uaa_token" -X POST https://api.system-domain.com/routing/v1/tcp_routes/create -d '
[{
  "router_group_guid": "ab1481be-d7f0-4390-6666-3e752e9f92ac",
  "port": 1121,
  "backend_ip": "10.0.8.5",
  "backend_port": 8888,
  "backend_sni_hostname":"teststst.asd.com",
  "ttl": 120
}]'
```

### Response
  Expected Status `201 CREATED`

Delete TCP Routes
-------------------
### Request
  `POST /routing/v1/tcp_routes/delete`

#### Request Headers
  A bearer token for an OAuth client with `routing.routes.write` scope is required.

#### Request Body
  A JSON-Encoded array of `TCP Route` objects for each route to delete.

| Object Field        | Type            | Required? | Description |
|---------------------|-----------------|-----------|-------------|
| `router_group_guid` | string          | yes       | GUID of the router group associated with this route.
| `port`              | integer         | yes       | External facing port for the TCP route.
| `backend_ip`        | string          | yes       | IP address of backend
| `backend_port`      | integer         | yes       | Backend port. Must be greater than 0.

#### Example Request
```bash
curl -vvv -H "Authorization: bearer [uaa token]" -X POST http://api.system-domain.com/routing/v1/tcp_routes/delete -d '
[{
  "router_group_guid": "xyz789",
  "port": 5200,
  "backend_ip": "10.1.1.12",
  "backend_port": 60000
}]'
```

### Response
  Expected Status `204 NO CONTENT`



Subscribe to Events for TCP Routes
-------------------
### Request
  `GET /routing/v1/tcp_routes/events`

#### Request Headers
  A bearer token for an OAuth client with `routing.routes.read` scope is required.

#### Example Request
```bash
curl -vvv -H "Authorization: bearer [uaa token]" http://api.system-domain.com/routing/v1/tcp_routes/events
```
### Response
  Expected Status `200 OK`

  The response is a long lived HTTP connection of content type
  `text/event-stream` as defined by
  https://www.w3.org/TR/2012/CR-eventsource-20121211/.

#### Example Response

```
id: 0
event: Upsert
data: {"router_group_guid":"xyz789","port":5200,"backend_port":60000,"backend_ip":"10.1.1.12","modification_tag":{"guid":"abc123","index":1},"ttl":120}

id: 1
event: Upsert
data: {"router_group_guid":"xyz789","port":5200,"backend_port":60000,"backend_ip":"10.1.1.12","modification_tag":{"guid":"abc123","index":2},"ttl":120}
```

List HTTP Routes (Experimental)
-------------------
Experimental -  subject to backward incompatible change

### Request
  `GET /routing/v1/routes`
#### Request Headers
  A bearer token for an OAuth client with `routing.routes.read` scope is required.

#### Example Request
```bash
curl -vvv -H "Authorization: bearer [uaa token]" http://api.system-domain.com/routing/v1/routes
```

### Response
  Expected Status `200 OK`

#### Response Body
  A JSON-encoded array of `HTTP Route` objects.

| Object Field        | Type            | Description |
|---------------------|-----------------|-------------|
| `route`             | string          | Address, including optional path, associated with one or more backends
| `ip`                | string          | IP address of backend
| `port`              | integer         | Backend port. Must be greater than 0.
| `ttl`               | integer         | Time to live, in seconds. The mapping of backend to route will be pruned after this time.
| `log_guid`          | string          | A string used to annotate routing logs for requests forwarded to this backend.
| `route_service_url` | string          | When present, requests for the route will be forwarded to this url before being forwarded to a backend. If provided, this url must use HTTPS.
| `modification_tag`  | object          | See [Modification Tags](modification_tags.md).

#### Example Response
```json
[{
  "route": "myapp.com/somepath",
  "port": 3000,
  "ip": "1.2.3.4",
  "ttl": 120,
  "log_guid": "routing_api",
  "modification_tag": {
    "guid": "abc123",
    "index": 1164
  }
}]
```

#### Create HTTP Routes (Experimental)
Experimental -  subject to backward incompatible change

As routes have a TTL, clients must register routes periodically to keep them active.

### Request
  `POST /routing/v1/routes`
#### Request Headers
  A bearer token for an OAuth client with `routing.routes.write` scope is required.
#### Request Body
  A JSON-encoded array of `HTTP Route` objects for each route to register.

| Object Field        | Type            | Required? | Description |
|---------------------|-----------------|-----------|-------------|
| `route`             | string          | yes       | Address, including optional path, associated with one or more backends
| `ip`                | string          | yes       | IP address of backend
| `port`              | integer         | yes       | Backend port. Must be greater than 0.
| `ttl`               | integer         | yes       | Time to live, in seconds. The mapping of backend to route will be pruned after this time. It must be greater than 0 seconds and less than the configured value for max_ttl (default 120 seconds).
| `log_guid`          | string          | no        | A string used to annotate routing logs for requests forwarded to this backend.
| `route_service_url` | string          | no        | When present, requests for the route will be forwarded to this url before being forwarded to a backend. If provided, this url must use HTTPS.

#### Example Request
```bash
curl -vvv -H "Authorization: bearer [uaa token]" -X POST http://api.system-domain.com/routing/v1/routes -d '[{"route":"myapp.com/somepath", "ip":"1.2.3.4", "port":8089, "ttl":120}]'
```

### Response
  Expected Status `201 CREATED`

Delete HTTP Routes (Experimental)
-------------------
Experimental -  subject to backward incompatible change

### Request
  `DELETE /routing/v1/routes`
#### Request Headers
  A bearer token for an OAuth client with `routing.routes.write` scope is required.
#### Request Body
  A JSON-encoded array of `HTTP Route` objects for each route to delete.

| Object Field        | Type            | Required? | Description |
|---------------------|-----------------|-----------|-------------|
| `route`             | string          | yes       | Address, including optional path, associated with one or more backends
| `ip`                | string          | yes       | IP address of backend
| `port`              | integer         | yes       | Backend port. Must be greater than 0.
| `log_guid`          | string          | no        | A string used to annotate routing logs for requests forwarded to this backend.
| `route_service_url` | string          | no        | When present, requests for the route will be forwarded to this url before being forwarded to a backend. If provided, this url must use HTTPS.

#### Example Request
```bash
curl -vvv -H "Authorization: bearer [uaa token]" -X DELETE http://api.system-domain.com/routing/v1/routes -d '[{"route":"myapp.com/somepath", "ip":"1.2.3.4", "port":8089, "ttl":120}]'
```

### Response
  Expected Status `204 NO CONTENT`

Subscribe to Events for HTTP Routes (Experimental)
-------------------
Experimental -  subject to backward incompatible change

### Request
  `GET /routing/v1/events`

#### Request Headers
  A bearer token for an OAuth client with `routing.routes.read` scope is required.

#### Example Request
```bash
curl -vvv -H "Authorization: bearer [uaa token]" http://api.system-domain.com/routing/v1/events
```
### Response
  Expected Status `200 OK`

  The response is a long lived HTTP connection of content type
  `text/event-stream` as defined by
  https://www.w3.org/TR/2012/CR-eventsource-20121211/.

#### Example Response:

```
id: 13
event: Upsert
data: {"route":"myapp.com/somepath","port":3000,"ip":"1.2.3.4","ttl":120,"log_guid":"routing_api","modification_tag":{"guid":"abc123","index":1154}}

id: 14
event: Upsert
data: {"route":"myapp.com/somepath","port":3001,"ip":"1.2.3.5","ttl":120,"log_guid":"routing_api","modification_tag":{"guid":"abc123","index":1155}}
```
