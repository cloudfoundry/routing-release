# Cloud Foundry Routing [BOSH release]

This repo is a [BOSH release](https://github.com/cloudfoundry/bosh) that delivers HTTP and TCP routing for Cloud Foundry.

## Developer Workflow

When working on individual components of TCP Router, work out of the submodules under `src/`.

Run the individual component unit tests as you work on them using
[ginkgo](https://github.com/onsi/ginkgo).

Commits to this repo (including Pull Requests) should be made on the Develop branch.

### Get the code

1. Fetch release repo

    ```
    mkdir -p ~/workspace
    cd ~/workspace
    git clone https://github.com/cloudfoundry-incubator/routing-release.git
    cd routing-release/
    ```


1. Automate `$GOPATH` and `$PATH` setup

    This BOSH release doubles as a `$GOPATH`. It will automatically be set up for you if you have [direnv](http://direnv.net) installed.


    ```
    direnv allow
    ```

    If you do not wish to use direnv, you can simply `source` the `.envrc` file in the root of the release repo.  You may manually need to update your `$GOPATH` and `$PATH` variables as you switch in and out of the directory.


1. Initialize and sync submodules


    ```
    ./scripts/update
    ```

### Running Unit Tests

1. Install ginkgo

        go install github.com/onsi/ginkgo/ginkgo

2. Run unit tests

        ./scripts/run-unit-tests

## Deploying to BOSH-Lite

### Prerequisites

1. Install and start [BOSH Lite](https://github.com/cloudfoundry/bosh-lite). Instructions can be found on that repo's README.
- Upload the latest Warden Trusty Go-Agent stemcell to BOSH Lite. You can download it first if you prefer.

	```
	bosh upload stemcell https://bosh.io/d/stemcells/bosh-warden-boshlite-ubuntu-trusty-go_agent
	```
- Install spiff, a tool for generating BOSH manifests. spiff is required for running the scripts in later steps. Stable binaries can be downloaded from [Spiff Releases](https://github.com/cloudfoundry-incubator/spiff/releases).
- Deploy [cf-release](https://github.com/cloudfoundry/cf-release) and [diego-release](https://github.com/cloudfoundry-incubator/diego-release).

> **Note**: for IAAS other than BOSH Lite, cf-release must be configured so
> that UAA terminates SSL; see [Deploying to Other
> IAAS](#deploying-to-other-iaas).

### Upload Release, Generate Manifest, and Deploy
1. Clone this repo and sync submodules; see [Get the code](#get-the-code).
- Upload routing-release to BOSH and generate a deployment manifest

    * Deploy the latest final release (master branch)

            cd ~/workspace/routing-release
            bosh -n upload release releases/routing-<lastest_version>.yml
            ./scripts/generate-bosh-lite-manifest
            bosh -n deploy

    * Or deploy from some other branch. The `release-candidate` branch can be considered "edge" as it has passed tests. The `update` script handles syncing submodules, among other things.

            cd ~/workspace/routing-release
            git checkout release-candidate
            ./scripts/update
            bosh create release --force
            bosh -n upload release
            ./scripts/generate-bosh-lite-manifest
            bosh -n deploy

	The `generate-bosh-lite-manifest` script expects the cf-release and diego-release manifests to be at `~/workspace/cf-release/bosh-lite/deployments/cf.yml` and `~/workspace/diego-release/bosh-lite/deployments/diego.yml`; the BOSH Lite manifest generation scripts for those releases will put them there by default. If cf and diego manifests are in a different location then you may specify them as arguments:

        ./scripts/generate-bosh-lite-manifest <cf_deployment_manifest> <diego_deployment_manifest>

> **Note**: for IAAS other than BOSH Lite, consider whether the reservable port
> range should be modified; see [Deploying to Other
> IAAS](#deploying-to-other-iaas).

### Redeploy cf-release to Enable the Routing API

Finally, update your cf-release deployment to enable the Routing API included in this release.

1. If you have a stub for overriding manifest properties of cf-release, add the following properties to this file. A [default one](bosh-lite/stubs/cf/routing-and-diego-enabled-overrides.yml) is provided. When you re-generate the manifest, these values will override the defaults in the manifest.

	```
	properties:
	  cc:
	    default_to_diego_backend: true
	  routing_api:
	    enabled: true
	```
	Though not strictly required, we recommend configuring Diego as your default backend (as configured with `default_to_diego_backend: true` above, as TCP Routing is only supported for Diego).
- Then generate a new manifest for cf-release and re-deploy it.

		cd ~/workspace/cf-release
		./scripts/generate-bosh-lite-dev-manifest ~/workspace/routing-release/bosh-lite/stubs/cf/routing-and-diego-enabled-overrides.yml  # or <path-to-your-stub>
		bosh -n deploy

### Post Deploy Configuration

Now that the release is deployed, you need to create a Shared Domain in CF and associate it with the TCP router group deployed with this release.

The CLI commands below require version 6.17+ of the [cf CLI](https://github.com/cloudfoundry/cli), and must be run as admin.

1. List available router-groups

	```
	$ cf router-groups
	Getting router groups as admin ...

	name          type
	default-tcp   tcp
	```

> **Note**: If you receive this error: `FAILED This command requires the
> Routing API. Your targeted endpoint reports it is not enabled`. This is due
> to the CF CLI's `~/.cf/config.json` having an old cached `RoutingEndpoint`
> value. To fix this, just do a cf login again and this error should go away.

- Create a shared-domain for the TCP router group

	```
	$ cf create-shared-domain tcp.bosh-lite.com --router-group default-tcp
	Creating shared domain tcp.bosh-lite.com as admin...
	OK
	```

> **Note**: For IAAS other than BOSH Lite, you will need to update a quota to
> grant permission for creating TCP routes. See [Deploying to Other
> IAAS](#deploying-to-other-iaas)

- Update the default quota to allow creation of unlimited TCP Routes

	Get the guid of the default org quota
	```
	$ cf curl /v2/quota_definitions?q=name:default
	```
	Update this quota definition to set `"total_reserved_route_ports": -1`
	```
	$ cf curl /v2/quota_definitions/44dff27d-96a2-44ed-8904-fb5ca8cbb298 -X PUT -d '{"total_reserved_route_ports": -1}'
	```

### Create a TCP Route

The CLI commands below require version 6.17+ of the [cf CLI](https://github.com/cloudfoundry/cli), and can be run as a user with the SpaceDeveloper role.

1. The simplest way to test TCP Routing is by pushing your app. By specifying the TCP domain and including the `--random-route` option, a TCP route will be created with a reserved port and the route mapped to your app.

	`$ cf p myapp -d tcp.bosh-lite.com --random-route`
- Send a request to your app using the TCP shared domain and the port reserved for your route.

	```
	$ curl tcp.bosh-lite.com:60073
	OK!
	```



## Deploying to other IAAS

BOSH Lite is a single VM environment intended for development. When deploying this release alongside Cloud Foundry in a distributed configuration, where jobs run on their own VMs, consider the following.

### UAA configuration

UAA needs to be configured with correct hostname so that routing components can
contact it. If you are using the manifest generation scripts for cf-release, the
following properties have been enabled by default. However, if you override the
`uaa.zones.internal.hostnames` property yourself, make sure to include `uaa.service.cf.internal`
in your stub.

```
properties:
  uaa:
    zones:
      internal:
        hostnames:
        - uaa.service.cf.internal
```

### UAA SSL must be enabled before deploying this release

The BOSH Lite manifest generation scripts use templates that have enabled the following properties by default. When generating a manifest for any other environment, you'll need to update your cf-release deployment with these manifest properties before generating the manifest for this release. This release's manifest generation scripts pull the value of `uaa.ssl.port` from the cf-release manifest.

		properties:
		  uaa:
		    ssl:
		      port: <choose a port for UAA to listen to SSL on; e.g. 8443>
		    sslCertificate: |
		      <insert certificate>
		    sslPrivateKey: |
		      <insert private key>

### OAuth clients in UAA

The following clients must be configured in UAA. If you're using the manifest generation scripts for cf-release, you can skip this step as the necessary clients are in the Spiff templates. If you're handrolling your manifest for cf-release, you'll need to add them.

```
properties:
  uaa:
    clients:
      cc_routing:
        authorities: routing.router_groups.read
        authorized-grant-types: client_credentials
        secret: <your-secret>
      gorouter:
        authorities: routing.routes.read
        authorized-grant-types: client_credentials,refresh_token
        secret: <your-secret>
      tcp_emitter:
        authorities: routing.routes.write,routing.routes.read,routing.router_groups.read
        authorized-grant-types: client_credentials,refresh_token
        secret: <your-secret>
      tcp_router:
        authorities: routing.routes.read,routing.router_groups.read
        authorized-grant-types: client_credentials,refresh_token
        secret: <your-secret>
```

### Horizontal Scalability

The TCP Router, TCP Emitter, and Routing API are stateless and horizontally scalable. Routing API depends on a clustered etcd data store. For high availability, deploy multiple instances of each job, distributed across regions of your infrastructure.

### Configuring Your Load Balancer to Health Check TCP Routers

In order to determine whether TCP Router instances are eligible for routing requests to, configure your load balancer to periodically check the health of each instance by attempting a TCP connection. By default the health check port is 80. This port can be configured using the `haproxy.health_check_port` property in the `property-overrides.yml` stub file.

To simulate this health check manually:
  ```
  nc -vz <tcp router IP> 80
  Connection to <tcp router IP> port 80 [tcp/http] succeeded!
  ```

### Configuring Port Ranges and DNS
1. Configure your load balancer to forward a range of ports to the IPs of the
   TCP Router instances. By default this release assumes 100 ports will be forwarded, in the range 1024-1123. The number of ports in the range dictates how many TCP routes can be created.
- If you configured your load balancer to forward a range other than 1024-1123, you must
  configure this release with the same port range using deployment
  manifest property `routing-api.router_groups.reservable_ports`, or [use the Routing API](https://github.com/cloudfoundry-incubator/routing-api#using-the-api-manually) (see "To update a Router Group's reservable_ports field with a new port range").
- Configure DNS to resolve a domain name to the load balancer. This domain name will be used by developers to create TCP routes for their applications. 
- After deploying this release you must add the domain you chose to CF as a
  Shared Domain (admin only), associating it with the Router Group.
  ```
  $ cf router-groups
  Getting router groups as admin ...

  name          type
  default-tcp   tcp

  $ cf create-shared-domain tcp.cfapps.example.com --router-group default-tcp
  ```

A Router Group represents a horizontally scalable cluster of identically configured routers. Only one router group is currently supported. Shared domains in Cloud Foundry are associated with one router group; see [Post Deploy Configuration](#post-deploy-configuration). To create a TCP route for their application, a developer creates it from a TCP domain; see [Create a TCP Route](#create-a-tcp-route). For each TCP route, Cloud Foundry reserves a port on the CF router. Each port is dedicated to that route; route ports may not be shared by multiple routes. The number of ports available for reservation dictates how many TCP routes can be created. 

A router group is limited to maximum port range 1024-65535; defaulting to 1024-1123. As one router group is supported,  the maximum number of TCP routes than can be created in CF is 64512 (65535-1024). Though multiple Shared Domains can be assigned to the router group, they share a common pool of ports. E.g. given Shared Domains `tcp-1.example.com` and `tcp-2.example.com are assigned to the `default-tcp` router group, and a route for port 1024 is created from domain `tcp-1.example.com`, the same port could not be reserved for a route from domain `tcp-2.example.com`. Eventually we may
support multiple router groups and/or TCP routes that share a port. 

The same reservable port range must be configured both on the load balancer, and in this release using the manifest property `routing-api.router_groups.reservable_ports` or the or [the Routing API](https://github.com/cloudfoundry-incubator/routing-api#using-the-api-manually). 

## Running Acceptance tests

### Using a BOSH errand on BOSH-Lite

Before running the acceptance tests errand, make sure to have the following setup.

1. bosh is targeted to your local bosh-lite
1. routing-release [deployed](#deploying-tcp-router-to-a-local-bosh-lite-instance) on bosh-lite

Run the following commands to execute the acceptance tests as an errand on bosh-lite

```
bosh run errand router_acceptance_tests
```

### Manually
See the README for [Routing Acceptance Tests](https://github.com/cloudfoundry-incubator/routing-acceptance-tests)

## Routing API
For details refer to [Routing API](https://github.com/cloudfoundry-incubator/routing-api/blob/master/README.md).

## TCP Router demo
For step by step instructions on TCP router demo done at Cloud Foundry Summit 2016, refer to [TCP Router demo](docs/demo.md)

## Metrics Documentation
For documentation on metrics available for streaming from Routing components
through the Loggregator
[Firehose](https://docs.cloudfoundry.org/loggregator/architecture.html), visit
the [CloudFoundry
Documentation](http://docs.cloudfoundry.org/loggregator/all_metrics.html#routing).
You can use the [NOAA Firehose sample app](https://github.com/cloudfoundry/noaa)
to quickly consume metrics from the Firehose.

## Gorouter Support for PROXY Protocol

Steps for enabling PROXY Protocol on the GoRouter can be found [here](https://github.com/cloudfoundry/gorouter/blob/master/README.md#proxy-protocol).

