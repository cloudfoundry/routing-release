These are historical instructions for deploying Routing for Cloud Foundry.
Routing is built into
[cf-deployment](https://github.com/cloudfoundry/cf-deployment) and no longer
needs to be installed separately.

## Deploying Routing for Cloud Foundry

1. For [high availability](#high-availability), configure a load balancer in
   front of the routers. If high-availability is not required allocate a public
   IP to a single instance of each router. See [Port Requirements for TCP
   Routing](#port-requirements-for-tcp-routing).

1. If you are using a load balancer see [Configuring Load Balancer
   Healthcheck](https://docs.cloudfoundry.org/adminguide/configure-lb-healthcheck.html).

1. Choose domain names from which developers will configure HTTP and TCP routes
   for their applications. Separate domain names will be required for HTTP and
   TCP routing. Configure DNS to resolve these domain names to the load balancer
   in front of the routers. You may use the same or separate load balancers for
   the HTTP and TCP domains. If high-availability is not required configure DNS
   to resolve the domains directly to a single instance of the routers.

1. If your manifest is configured with self-signed certificates for UAA,
   configure routing components to skip validation of the TLS certificate; see
   [Validation of TLS Certificates from Route Services and
   UAA](#validation-of-tls-certificates-from-route-services-and-uaa).

1. Deploy Cloud Foundry using the instructions for
   [cf-deployment](https://github.com/cloudfoundry/cf-deployment/blob/master/deployment-guide.md).

### Port Requirements for TCP Routing

Choose how many TCP routes you'd like to offer. For each TCP route, a port must
be opened on your load balancer. Configure your load balancer to forward the
range of ports you choose to the IP addresses of the TCP Router instances.

Routing API must be configured with the same range of ports. By default
`cf-deployment` will enable 100 ports in the range 1024-1123. If you choose a
range other than 1024-1123, you must configure this using the deployment
manifest property `routing-api.router_groups.reservable_ports`. This is a seeded
value only; after deploy, changes to this property will be ignored. To modify
the reservable port range after deployment, use the [Routing
API](https://github.com/cloudfoundry/routing-api#using-the-api-manually); (see
"To update a Router Group's reservable_ports field with a new port range").

```yaml
- name: routing_api
  properties:
    routing_api:
    router_groups:
    - name: default-tcp
      reservable_ports: 1024-1123
      type: tcp
    - name: test1
      reservable_ports:
        - 1066
        - 1266
      type: tcp
    - name: test2
      reservable_ports: 1111-2222,4444
      type: tcp

```

### Validation of TLS Certificates from Route Services and UAA

The following components communicate with UAA via TLS:
- Routing API
- GoRouter
- TCP Router

Additionally, GoRouter communicates with [Route
Services](http://docs.cloudfoundry.org/services/route-services.html) via TLS.

In all cases, these components will validate that certificates are signed by a
known CA and that they match the requested domains. To disable this validation,
as when deploying the routing subsystem to an environment with self-signed
certificates, configure the `skip_ssl_validation` property for GoRouter,
Routing API and TCP Router.


```yaml
- name: routing-api
  properties:
    skip_ssl_validation: true
- name: tcp_router
  properties:
    skip_ssl_validation: true
- name: gorouter
  properties:
    router:
      ssl_skip_validation: true
```

## Post Deploy Steps

### Create a Shared Domain in CF

  After deploying this release you must add the domain you chose (see [Domain
  Names](#domain-names)) to CF as a Shared Domain, associating it
  with the Router Group.

  The CLI commands below require version 6.17+ of the [`cf`
  CLI](https://github.com/cloudfoundry/cli), and must be run as `admin`.

  List available router-groups

  ```
  $ cf router-groups
  Getting router groups as admin ...

  name          type
  default-tcp   tcp
  ```

  **Note**: If you receive this error: `FAILED This command requires the
  Routing API. Your targeted endpoint reports it is not enabled`. This is due
  to the CF CLI's `~/.cf/config.json` having an old cached `RoutingEndpoint`
  value. To fix this, just do a `cf login` again and this error should go away.

  Create a shared-domain for the TCP router group

  ```
  $ cf create-shared-domain tcp-apps-domain.com --router-group default-tcp
  Creating shared domain tcp-apps-domain.com as admin...
  OK
  ```

  See [Router Groups](#router-groups) for details on that concept.

## Manual Configuration of BOSH Deployment Manifest

If you use the canonical manifest provided with cf-deployment the following prerequisites will be configured
for you automatically.

### UAA

1. UAA must be configured to terminate TLS for internal requests. Set the
   following properties in your environment stub for cf-release when using the
   manifest generation scripts, or set it directly in your manifest. The
   routing-release's manifest generation scripts will set `uaa.tls_port` to the
   value of `uaa.ssl.port` from the cf-release manifest.

    ```
    - name: uaa
      properties:
        uaa:
          ssl:
            port: <choose a port for UAA to listen to SSL on; e.g. 8443>
          sslCertificate: |
            <insert certificate>
          sslPrivateKey: |
            <insert private key>
    ```
1. You must add the `routing.router_groups.read` and
  `routing.router_groups.write` scopes to your admin user.

    ```
    - name: uaa
      properties:
        uaa:
          scim:
            users:
            - name: admin
              password: PASSWORD
              groups:
              - scim.write
              - scim.read
              - openid
              - cloud_controller.admin
              - clients.read
              - clients.write
              - doppler.firehose
              - routing.router_groups.read
              - routing.router_groups.write
    ```

1. The following OAuth clients must be configured for UAA. All but the `cf`
   client are new; the important change to the `cf` client is adding the
   `routing.router_groups.read` and `routing.router_groups.write` scopes. If
   you're using the manifest generation scripts for cf-release, you can skip
   this step as the necessary clients are in the Spiff templates. If you're
   handrolling your manifest for cf-release, you'll need to add them.

    ```
    - name: uaa
      properties:
        uaa:
          clients:
            cc_routing:
              authorities: routing.router_groups.read
              authorized-grant-types: client_credentials
              secret: <your-secret>
            cf:
              override: true
              authorized-grant-types: password,refresh_token
              scope: cloud_controller.read,cloud_controller.write,openid,password.write,cloud_controller.admin,cloud_controller.admin_read_only,scim.read,scim.write,doppler.firehose,uaa.user,routing.router_groups.read,routing.router_groups.write
              authorities: uaa.none
              access-token-validity: 600
              refresh-token-validity: 2592000
            gorouter:
              authorities: routing.routes.read
              authorized-grant-types: client_credentials,refresh_token
              secret: <your-secret>
            tcp_emitter:
              authorities: routing.routes.write,routing.routes.read
              authorized-grant-types: client_credentials,refresh_token
              secret: <your-secret>
            tcp_router:
              authorities: routing.routes.read
              authorized-grant-types: client_credentials,refresh_token
              secret: <your-secret>
    ```
1. UAA must be configured to accept requests using an internal hostname. The
   manifest generation scripts for cf-release will do this for you (both BOSH
   Lite and non). However, if you override the `uaa.zones.internal.hostnames`
   property yourself, be sure to include `uaa.service.cf.internal` in your
   stub.

   ```
   - name: uaa
     properties:
       uaa:
          internal_url: https://uaa.service.cf.internal:8443
          ca_cert: "((uaa_ca.certificate))"
   ```

### Routing API Database

1. Routing API requires a relational database as a data store; MySQL and PostgreSQL are supported. Routing API does not create the database; one must exist on startup. cf-deployment uses [CF MySQL Release](https://github.com/cloudfoundry/cf-mysql-release) by default. For any deployment you can use this or provide your own MySQL or PostgreSQL database. Configure the credentials for the database with the following properties.

  ```
  - name: routing-api
    properties:
      routing_api:
      sqldb:
        type: <mysql || postgres>
        host: <IP of SQL Host>
        port: <Port for SQL Host>
        schema: <Schema name>
        username: <Username for SQL DB>
        password: <Password for SQL DB>
  ```

  If you are using [cf-mysql-release](https://github.com/cloudfoundry/cf-mysql-release), then the values for these properties can be obtained from the following properties in the manifest for that release.
    - `type` should be `mysql`
    - `host` corresponds to the IP address of the `proxy_z1` job
    - `port` is `3306`
    - `schema` corresponds to `cf_mysql.mysql.seeded_databases[].name`
    - `username` corresponds to `cf_mysql.mysql.seeded_databases[].username`
    - `password` corresponds to `cf_mysql.mysql.seeded_databases[].password`

1.  If you use the CF MySQL Release, you can seed the required database on deploy using the manifest property `cf_mysql.mysql.seeded_databases`. Do not use the same deployment of cf-mysql-release that is exposed as a CF marketplace service. Instead, use a deployment intended for internal platform use; for these deployments you should set broker instances to zero. Update the following manifest properties before deploying.

  ```
  - name: database
    properties:
      cf_mysql:
        mysql:
          seeded_databases:
          - name: routing-api
            username: <your-username>
            password: <your-password>
  ```

### Enable Support for TCP Routing in Route Emitter

The Diego Route Emitter component must be configured to emit TCP routes in order to support TCP routing.

  ```
  - name: route_emitter
    properties:
     tcp:
       enabled: true
  ```

## Router Groups

A Router Group represents a horizontally scalable cluster of identically
configured routers. Only one router group is currently supported. Shared
domains in Cloud Foundry are associated with one router group; see [Post Deploy
Configuration](#post-deploy-configuration). To create a TCP route for their
application, a developer creates it from a TCP domain; see [Create a TCP
Route](#create-a-tcp-route). For each TCP route, Cloud Foundry reserves a port
on the CF router. Each port is dedicated to that route; route ports may not be
shared by multiple routes. The number of ports available for reservation
dictates how many TCP routes can be created.

A router group is limited to maximum port range 1024-65535; defaulting to
1024-1123. As one router group is supported,  the maximum number of TCP routes
than can be created in CF is 64512 (65535-1024). Though multiple Shared Domains
can be assigned to the router group, they share a common pool of ports. E.g.
given Shared Domains `tcp-1.example.com` and `tcp-2.example.com` are assigned to
the `default-tcp` router group, and a route for port 1024 is created from
domain `tcp-1.example.com`, the same port could not be reserved for a route
from domain `tcp-2.example.com`. Eventually we may support multiple router
groups and/or TCP routes that share a port.

The same reservable port range must be configured both on the load balancer,
and in this release using the manifest property
`routing-api.router_groups.reservable_ports`. The port range can be modified
after deploying using [the Routing
API](https://github.com/cloudfoundry/routing-api#using-the-api-manually).

**Note:** when modifying the port range using the Routing API, consider that
the new port range must include those ports that have already been reserved.

