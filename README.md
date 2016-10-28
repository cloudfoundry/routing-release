# Cloud Foundry Routing [BOSH release]

This repo is a [BOSH release](https://github.com/cloudfoundry/bosh) that
delivers HTTP and TCP routing for Cloud Foundry.

## Developer Workflow

When working on individual components of TCP Router, work out of the submodules
under `src/`.

Run the individual component unit tests as you work on them using
[ginkgo](https://github.com/onsi/ginkgo).

Commits to this repo (including Pull Requests) should be made on the Develop
branch.

### Get the code

1. Fetch release repo.

  ```
  mkdir -p ~/workspace
  cd ~/workspace
  git clone https://github.com/cloudfoundry-incubator/routing-release.git
  cd routing-release/
  ```

1. Automate `$GOPATH` and `$PATH` setup.

  This BOSH release doubles as a `$GOPATH`. It will automatically be set up
  for you if you have [direnv](http://direnv.net) installed.

  ```
  direnv allow
  ```

  If you do not wish to use direnv, you can simply `source` the `.envrc` file
  in the root of the release repo.  You may manually need to update your
  `$GOPATH` and `$PATH` variables as you switch in and out of the directory.

1. Initialize and sync submodules.

  ```
  ./scripts/update
  ```

### Running Unit Tests

1. Install ginkgo

  ```
  go install github.com/onsi/ginkgo/ginkgo
  ```

2. Run unit tests

  ```
  ./scripts/run-unit-tests
  ```

## Deployment Prerequisites

1. [Deploy BOSH](http://bosh.io/docs). An easy way to get started using a
   single VM is BOSH Lite.

1. Upload the latest Warden Trusty Go-Agent stemcell for your IaaS to the BOSH
   Director. Stemcells can be found at [bosh.io](https://bosh.io). You can
   download it first if you prefer.

    ```
    bosh upload stemcell https://bosh.io/d/stemcells/bosh-warden-boshlite-ubuntu-trusty-go_agent
    ```

1. Install spiff, a tool for generating BOSH manifests. spiff is required for
   running the scripts in later steps. Stable binaries can be downloaded from
   [Spiff Releases](https://github.com/cloudfoundry-incubator/spiff/releases).

1. Deploy [cf-release](https://github.com/cloudfoundry/cf-release),
   [diego-release](https://github.com/cloudfoundry-incubator/diego-release).
   For IAAS other than BOSH Lite, this release requires specific configuration
   for cf-release; see [Prerequisite Configuration](#prerequisite-configuration).

1. Configure a load balancer providing high availability for the TCP routers to
   forward a range of ports to the TCP routers. See
   [Load Balancer Requirements](#load-balancer-requirements).

1. Choose a domain name for developer to create TCP route from and configure
   DNS to resolve it to your load balancer; see [Domain Names](#domain-names).

### CF-Release

If you use the BOSH Lite manifest generation script cf-release, and deploy the
latest release of cf-release, the following prerequisites will be configured
for you automatically.

1. UAA must be configured to terminate TLS for internal requests. Set the
   following properties in your environment stub for cf-release when using the
   manifest generation scripts, or set it directly in your manifest. The
   routing-release's manifest generation scripts will set `uaa.tls_port` to the
   value of `uaa.ssl.port` from the cf-release manifest.

    ```
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
    properties:
      uaa:
      scim:
	  users:
	  - admin|PASSWORD|scim.write,scim.read,openid,cloud_controller.admin,clients.read,clients.write,doppler.firehose,routing.router_groups.read,routing.router_groups.write
    ```

1. The following OAuth clients must be configured for UAA. All but the `cf`
   client are new; the important change to the `cf` client is adding the
   `routing.router_groups.read` and `routing.router_groups.write` scopes. If
   you're using the manifest generation scripts for cf-release, you can skip
   this step as the necessary clients are in the Spiff templates. If you're
   handrolling your manifest for cf-release, you'll need to add them.

    ```
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
   properties:
     uaa:
       zones:
         internal:
           hostnames:
           - uaa.service.cf.internal
   ```

### Relational Database

This release supports a relational database as a data store for the Routing API; MySQL and PostgreSQL are supported. BOSH Lite deployments will use the PostgreSQL database that comes with cf-release by default. For other IaaS we recommend the [CF MySQL Release](https://github.com/cloudfoundry/cf-mysql-release). For any deployment, you can also provide your own MySQL or PostgreSQL database. Routing API does not create the database on deployment of routing-release; you must create a database schema in advance and then provide the credentials for it in the deployment manifest for this release (see [Deploying routing-release](#deploying-routing-release)). 

For the CF MySQL Release, you can seed the required database on deploy using the manifest property `cf_mysql.mysql.seeded_databases`. We recommend you do not use a deployment of cf-mysql-release that is exposed as a CF marketplace service. Instead, use a deployment intended for internal platform use; for these deployments you should set broker instances to zero. After generating your manifest for cf-mysql-release, update the following manifest properties before deploying.

```
properties:
  cf_mysql:
    mysql:
      seeded_databases:
      - name: routing-api
        username: <your-username>
        password: <your-password>
...
jobs:
- cf-mysql-broker_z1
  instances: 0
...
- cf-mysql-broker_z2
  instances: 0
```

### Load Balancer requirements for TCP routing

If you are deploying routing-release to an environment that requires high availability, a load balancer is required to front the TCP routers. The HAProxy job that comes with cf-release does not fulfill this requirement. If you are using a load balancer for this purpose, you must configure it to forward a range of ports to the TCP routers, and also to periodically healthcheck them. If high-availability is not required you can skip this section and allocate a public IP to a single TCP router instance.

For more on high availability, see [High Availability](#high-availability).

#### Ports

Choose how many TCP routes you'd like to offer. For each TCP route, a port must
be opened on your load balancer. Configure your load balancer to forward the
range of ports you choose to the IPs of the TCP Router instances. By default
this release assumes 100 ports will be forwarded, in the range 1024-1123.

#### Healthchecking of TCP Routers

In order to determine whether TCP Router instances are eligible for routing
requests to, configure your load balancer to periodically check the health of
each instance by attempting a TCP connection. By default the health check port
is 80. This port can be configured using the `haproxy.health_check_port`
property in the `property-overrides.yml` stub file.

For example, to simulate this health check manually:
```
nc -vz <tcp router IP> 80
Connection to <tcp router IP> port 80 [tcp/http] succeeded!
```

You can also check the health of each TCP Router instance by making an HTTP
request to `http://<tcp router IP>:<haproxy.health_check_port>/health`.

For example:
```
curl http://<tcp router IP>:80/health
<html><body><h1>200 OK</h1>
Service ready.
</body></html>
```
### Domain Names

Choose a domain name from which developers will configure TCP routes for their
applications. Configure DNS to resolve this domain name to the load balancer.
If high-availability is not required configure DNS to resolve the TCP domain
directly to a single TCP router instance.

## Deploying routing-release

1. Clone this repo and sync submodules; see [Get the code](#get-the-code).
1. Upload routing-release to BOSH

    - Latest final release (master branch)

      ```
      cd ~/workspace/routing-release
      bosh -n upload release releases/routing-<lastest_version>.yml
      ```

    - The `release-candidate` branch can be considered "edge" as it has passed
      tests. The `update` script handles syncing submodules, among other
      things.

      ```
      cd ~/workspace/routing-release
      git checkout release-candidate
      ./scripts/update
      bosh create release --force
      bosh -n upload release
      ```

1. Generate a Deployment Manifest

	The following scripts can be used to generate manifests for your deployment.

	- For BOSH Lite: `./scripts/generate-bosh-lite-manifest`
	- For other IaaS: `./scripts/generate-manifest`

	Both scripts support the following options:
	
	- `-c` path to cf-release-manifest
	- `-d` path to diego-release-manifest
	- `-l` list of stubs

	If no options are provided, the `generate-bosh-lite-manifest` script expects the cf-release and diego-release manifests to be at `~/workspace/cf-release/bosh-lite/deployments/cf.yml` and `~/workspace/diego-release/bosh-lite/deployments/diego.yml`; the BOSH Lite manifest generation scripts for those releases will put them there by default. 

	**Important:** Before deploying, consider the following sections on manifest configuration for reservable TCP ports and relational databases, and on upgrading to a relational database from etcd.  

1. Deploy

	```
	bosh deploy
	```

### Ports

If you configured your load balancer to forward a range other than
1024-1123 (see [Ports](#ports)), you must configure this release with the
same port range using deployment manifest property
`routing-api.router_groups.reservable_ports`. This is a seeded value only;
after deploy, changes to this property will be ignored. To modify the
reservable port range after deployment, use the [Routing
API](https://github.com/cloudfoundry-incubator/routing-api#using-the-api-manually);
(see "To update a Router Group's reservable_ports field with a new port
range").

```
properties:
  routing_api:
	router_groups:
	- name: default-tcp
	  reservable_ports: 1024-1123
	  type: tcp
```

### Relational Database
	
The routing-release now supports a relational database for the Routing API. We recommend this instead of etcd. To opt into this feature you can configure your manifest stub with the following `sqldb` properties. To migrate existing deployments to use a relational database see [Migrating from ETCD](#Migrating from ETCD)

```
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

If you are using
[cf-mysql-release](https://github.com/cloudfoundry/cf-mysql-release), then
the values for these properties can be obtained from properties that
manifest.
  - `type` should be `mysql`
  - `host` corresponds to the IP address of the `proxy_z1` job
  - `port` is `3306`
  - `schema` corresponds to `cf_mysql.mysql.seeded_databases[].name`
  - `username` corresponds to `cf_mysql.mysql.seeded_databases[].username`
  - `password` corresponds to `cf_mysql.mysql.seeded_databases[].password`

### Migrating from etcd

For existing deployments that use etcd, there is a two-phase upgrade process
to migrate to a relational database.
1. Deploy the most recent version of routing-release. The migration depends on a recent change to routing-api whereby only one instance is active at a time; this is achieved using a lock in Consul.
1. Configure your manifest with the `routing_api.sqldb` property and redeploy routing-release.

This process should ensure a migration with zero downtime to application backends.

## Post Deploy Steps

1. Redeploy cf-release to Enable the Routing API

  After deploying routing-release, you must update your cf-release deployment
  to enable the Routing API included in this release.

  If you have a stub for overriding manifest properties of cf-release,
  add the following properties to this file. A [default
  one](bosh-lite/stubs/cf/routing-and-diego-enabled-overrides.yml) is
  provided. When you re-generate the manifest, these values will override
  the defaults in the manifest.

  ``` properties:
        cc:
          default_to_diego_backend: true
        routing_api:
          enabled: true
  ```

  Though not strictly required, we recommend configuring Diego as your default
  backend (as configured with `default_to_diego_backend: true` above, as TCP
  Routing is only supported for Diego).

  Then generate a new manifest for cf-release and re-deploy it.

  ```
  cd ~/workspace/cf-release
  ./scripts/generate-bosh-lite-dev-manifest ~/workspace/routing-release/bosh-lite/stubs/cf/routing-and-diego-enabled-overrides.yml  # or <path-to-your-stub>
  bosh -n deploy
  ```

1. Create a Shared Domain in CF

  After deploying this release you must add the domain you chose (see [Domain
  Names](#domain-names)) to CF as a Shared Domain (admin only), associating it
  with the Router Group.

  The CLI commands below require version 6.17+ of the [cf
  CLI](https://github.com/cloudfoundry/cli), and must be run as admin.

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
  value. To fix this, just do a cf login again and this error should go away.

  Create a shared-domain for the TCP router group

  ```
  $ cf create-shared-domain tcp.bosh-lite.com --router-group default-tcp
  Creating shared domain tcp.bosh-lite.com as admin...
  OK
  ```

  See [Router Groups](#router-groups) for details on that concept.

1. Enable Quotas for TCP Routing

  As ports can be a limited resource in some environments, the default quotas
  in Cloud Foundry for IaaS other than BOSH Lite do not allow reservation of
  route ports; required for creation of TCP routes. The final step to enabling
  TCP routing is to modify quotas to set the maximum number of TCP routes that
  may be created by each organization or space.

  Get the guid of the default org quota

  ```
  $ cf curl /v2/quota_definitions?q=name:default
  ```
  Update this quota definition to set `"total_reserved_route_ports": -1`

  ```
  $ cf curl /v2/quota_definitions/44dff27d-96a2-44ed-8904-fb5ca8cbb298 -X PUT -d '{"total_reserved_route_ports": -1}'
  ```

## Creating a TCP Route

The CLI commands below require version 6.17+ of the [cf
CLI](https://github.com/cloudfoundry/cli), and can be run as a user with the
SpaceDeveloper role.

1. The simplest way to test TCP Routing is by pushing your app. By specifying
   the TCP domain and including the `--random-route` option, a TCP route will
   be created with a reserved port and the route mapped to your app.

    `$ cf p myapp -d tcp.bosh-lite.com --random-route`

1. Send a request to your app using the TCP shared domain and the port reserved for your route.

    ```
    $ curl tcp.bosh-lite.com:60073
    OK!
    ```

### TCP Router demo
For step by step instructions on TCP router demo done at Cloud Foundry Summit
2016, refer to [TCP Router demo](docs/demo.md)

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
given Shared Domains `tcp-1.example.com` and `tcp-2.example.com are assigned to
the `default-tcp` router group, and a route for port 1024 is created from
domain `tcp-1.example.com`, the same port could not be reserved for a route
from domain `tcp-2.example.com`. Eventually we may support multiple router
groups and/or TCP routes that share a port.

The same reservable port range must be configured both on the load balancer,
and in this release using the manifest property
`routing-api.router_groups.reservable_ports`. The port range can be modified
after deploying using [the Routing
API](https://github.com/cloudfoundry-incubator/routing-api#using-the-api-manually).

**Note:** when modifying the port range using the Routing API, consider that
the new port range must include those ports that have already been reserved.

## High Availability

The TCP Router, TCP Emitter, and Routing API are stateless and horizontally
scalable. The TCP Routers must be fronted by a load balancer for
high-availability. The Routing API depends on a clustered etcd data store. For
high availability, deploy multiple instances of each job, distributed across
regions of your infrastructure.


## Running Acceptance tests

### Using a BOSH errand on BOSH-Lite

Before running the acceptance tests errand, make sure to have the following
setup.

1. bosh is targeted to your local bosh-lite
1. routing-release
   [deployed](#deploying-tcp-router-to-a-local-bosh-lite-instance) on bosh-lite
1. Endpoints for http routes are not tested by the errand by default. To enable
   them, set the property `properties.acceptance_tests.include_http_routes` in
   your manifest for the errand job.

Run the following commands to execute the acceptance tests as an errand on
bosh-lite

```
bosh run errand routing_acceptance_tests
```

### Manually
See the README for [Routing Acceptance Tests](https://github.com/cloudfoundry-incubator/routing-acceptance-tests)

## Routing API
For details refer to [Routing API](https://github.com/cloudfoundry-incubator/routing-api/blob/master/README.md).

## Metrics Documentation
For documentation on metrics available for streaming from Routing components
through the Loggregator
[Firehose](https://docs.cloudfoundry.org/loggregator/architecture.html), visit
the [CloudFoundry
Documentation](http://docs.cloudfoundry.org/loggregator/all_metrics.html#routing).
You can use the [NOAA Firehose sample app](https://github.com/cloudfoundry/noaa)
to quickly consume metrics from the Firehose.

## Gorouter Support for PROXY Protocol

Steps for enabling PROXY Protocol on the GoRouter can be found
[here](https://github.com/cloudfoundry/gorouter/blob/master/README.md#proxy-protocol).

## Configure Load Balancer healthchecks for Gorouter

- On shutdown, the healthcheck endpoint will return 503 for a duration defined
  by `router.drain_wait` while Gorouter continues to serve requests. When
  `router.drain_wait` expires, Gorouter will stop accepting connections and the
  process will gracefully stop.

- On startup:
  - If `router.load_balancer_healthy_threshold` ==
    `router.requested_route_registration_interval_in_seconds`, then healthcheck
    endpoint will return 200 immediately on start and server accept connections
    after a delay equal to either of these properties
  - If `router.load_balancer_healthy_threshold` <
    `router.requested_route_registration_interval_in_seconds`, then healthcheck
    will return 200 immediately on start and server will accept connections
    after a delay equal to `load_balancer_healthy_threshold`
  - If `router.load_balancer_healthy_threshold` >
    `router.requested_route_registration_interval_in_seconds`, then healthcheck
    will return 503 on start, then 200 OK after a delay equal to
    `requested_route_registration_interval_in_seconds -
    load_balancer_healthy_threshold`. Server will begin accepting connections
    after an additional delay equal to `load_balancer_healthy_threshold`.
