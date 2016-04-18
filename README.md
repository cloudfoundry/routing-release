# Cloud Foundry Routing [BOSH release]

This repo is a [BOSH](https://github.com/cloudfoundry/bosh) release for deploying TCP Router and associated tasks.

The TCP Router adds non-HTTP routing capabilities to Cloud Foundry.

## Developer Workflow

When working on individual components of TCP Router, work out of the submodules under `src/`.

Run the individual component unit tests as you work on them using
[ginkgo](https://github.com/onsi/ginkgo).

Commits to this repo (including Pull Requests) should be made on the Develop branch.

##<a name="get-the-code"></a> Get the code

1. Fetch release repo

    ```
    mkdir -p ~/workspace
    cd ~/workspace
    git clone https://github.com/cloudfoundry-incubator/cf-routing-release.git
    cd cf-routing-release/
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
    
## Running Unit Tests

1. Install ginkgo

        go install github.com/onsi/ginkgo/ginkgo

2. Run unit tests

        ./scripts/run-unit-tests

## Deploying to BOSH-Lite 

1. Install and start [BOSH Lite](https://github.com/cloudfoundry/bosh-lite). Instructions can be found on that repo's README.
- Upload the latest Warden Trusty Go-Agent stemcell to BOSH Lite. You can download it first if you prefer.

	```
	bosh upload stemcell https://bosh.io/d/stemcells/bosh-warden-boshlite-ubuntu-trusty-go_agent
	```
- Install spiff, a tool for generating BOSH manifests. spiff is required for running the scripts in later steps. Stable binaries can be downloaded from [Spiff Releases](https://github.com/cloudfoundry-incubator/spiff/releases).
- Deploy [cf-release](https://github.com/cloudfoundry/cf-release) and [diego-release](https://github.com/cloudfoundry-incubator/diego-release)
- Clone this repo and sync submodules; see [Get the code](#get-the-code).
- Upload cf-routing-release to BOSH and generate a deployment manifest

    * Deploy the latest final release (master branch)

            cd ~/workspace/cf-routing-release
            bosh -n upload release releases/cf-routing-<lastest_version>.yml
            ./scripts/generate-bosh-lite-manifest
            bosh -n deploy

    * Or deploy from some other branch. The `release-candidate` branch can be considered "edge" as it has passed tests. The `update` script handles syncing submodules, among other things.

            cd ~/workspace/cf-routing-release
            git checkout release-candidate
            ./scripts/update
            bosh create release --force
            bosh -n upload release
            ./scripts/generate-bosh-lite-manifest
            bosh -n deploy

	The `generate-bosh-lite-manifest` script expects the cf-release and diego-release manifests to be at `~/workspace/cf-release/bosh-lite/deployments/cf.yml` and `~/workspace/diego-release/bosh-lite/deployments/diego.yml`; the BOSH Lite manifest generation scripts for those releases will put them there by default. If cf and diego manifests are in a different location then you may specify them as arguments:

        ./scripts/generate-bosh-lite-manifest <cf_deployment_manifest> <diego_deployment_manifest>
- Finally, update your cf-release deployment to enable the Routing API included in this release.

	If you don't already have one, create a file for overriding manifest properties of cf-release. In the context of manifest generation, we call this file a stub; you could name it `cf-boshlite-stub.yml`. Add the following properties to this file. When you re-generate the manifest, these values will override the defaults in the manifest.

		properties:
		  cc:
		    default_to_diego_backend: true
		  routing_api:
		    enabled: true


	Though not strictly required, we recommend configuring Diego as your default backend, as TCP Routing is only supported for Diego.

	Then generate a new manifest for cf-release and re-deploy it.

		cd ~/workspace/cf-release
		./scripts/generate-bosh-lite-dev-manifest <path-to-your-stub>
		bosh -n deploy
- Create a shared domain for the TCP router group; see [Testing the TCP Routing manually](#testing-tcp-routing-manually)


## Deploying to other IAAS

BOSH Lite is a single VM environment intended for development. When deploying this release alongside Cloud Foundry in a distributed configuration, where jobs run on their own VMs, consider the following.

### UAA SSL must be enabled before deploying this release

The BOSH Lite manifest generation scripts use templates that have enabled the following properties by default. When generating a manifest for any other environment, you'll need to update your cf-release deployment with these manifest properties before deploying this release.

		properties:
		  uaa:
		    ssl:
		      port: <choose a port for UAA to listen to SSL on; e.g. 8443> 
		    sslCertificate: |
		      <insert certificate>
		    sslPrivateKey: | 
		      <insert private key>

### Horizontal Scalability

The TCP Router, TCP Emitter, and Routing API are stateless and horizontally scalable. Routing API depends on a clustered etcd data store. For high availability, deploy multiple instances of each job, distributed across regions of your infrastructure. 

### Configuring Your Load Balancer to Health Check TCP Routers

In order to determine whether TCP Router instances are eligible for routing requests to, configure your load balancer to periodically check the health of each instance by attempting a TCP connection. By default the health check port is 80. This port can be configured using the `haproxy.health_check_port` property in the property-overrides.yml stub file.

To simulate this health check manually on BOSH-lite:
  ```
  nc -z 10.244.14.2 80
  ```

## Running Acceptance tests

### Using a BOSH errand on BOSH-Lite

Before running the acceptance tests errand, make sure to have the following setup.

1. bosh is targeted to your local bosh-lite
1. cf-routing-release [deployed](#deploying-tcp-router-to-a-local-bosh-lite-instance) on bosh-lite 

Run the following commands to execute the acceptance tests as an errand on bosh-lite

```
bosh run errand router_acceptance_tests
```

### Manually 
See the README for [Routing Acceptance Tests](https://github.com/cloudfoundry-incubator/cf-routing-acceptance-tests)

## Testing TCP Routing manually

These instructions assume the release has been deployed to bosh-lite, along with cf-release and diego-release. See [Deploying cf-routing-release to a local BOSH-Lite instance](#deploying-tcp-router-to-a-local-bosh-lite-instance) above. The CLI commands below require version 6.17 of the [cf CLI](https://github.com/cloudfoundry/cli).

1. As admin, list available router-groups

	```
	$ cf router-groups
	Getting router groups as admin ...
	
	name          type
	default-tcp   tcp
	```
- As admin, create a shared-domain for the TCP router group

	```
	$ cf create-shared-domain tcp.bosh-lite.com --router-group default-tcp
	Creating shared domain tcp.bosh-lite.com as admin...
	OK
	```
- Push your app, specifying the TCP domain and `--random-route`. A TCP port will be reserved for you.

	`$ cf p myapp -d tcp.bosh-lite.com --random-route`
- Send a request to your app using the TCP shared domain and the port reserved for your route.

	```
	$ curl tcp.bosh-lite.com:60073
	OK!
	```

## Routing API

For details refer to [Routing API](https://github.com/cloudfoundry-incubator/routing-api/blob/master/README.md).
