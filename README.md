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

## Deploying cf-routing-release to a local BOSH-Lite instance

1. Install and start [BOSH Lite](https://github.com/cloudfoundry/bosh-lite). Instructions can be found on that repo's README.

1. Upload the latest Warden Trusty Go-Agent stemcell to BOSH Lite. You can download it first if you prefer.

	```
	bosh upload stemcell https://bosh.io/d/stemcells/bosh-warden-boshlite-ubuntu-trusty-go_agent
	```

1. Install spiff, a tool for generating BOSH manifests. spiff is required for running the scripts in later steps. Stable binaries can be downloaded from [Spiff Releases](https://github.com/cloudfoundry-incubator/spiff/releases).

1. Deploy [cf-release](https://github.com/cloudfoundry/cf-release) and [diego-release](https://github.com/cloudfoundry-incubator/diego-release). Instructions can be found on those repo's READMEs.

1. Clone this repo and sync submodules; see [Get the code](#get-the-code).

1. Upload cf-routing-release to BOSH and generate a deployment manifest

    * Deploy the latest final release (master branch)

            cd ~/workspace/cf-routing-release
            bosh -n upload release releases/cf-routing-<lastest_version>.yml
            ./scripts/generate-bosh-lite-manifest
            bosh -n deploy

    * Or deploy from some other branch. The `release_candidate` branch can be considered "edge" as it has passed tests. The `update` script handles syncing submodules, among other things.

            cd ~/workspace/cf-routing-release
            git checkout release_candidate
            ./scripts/update
            bosh create release --force
            bosh -n upload release
            ./scripts/generate-bosh-lite-manifest
            bosh -n deploy

	The `generate-bosh-lite-manifest` script expects `cf.yml` to be present in `~/workspace/cf-release/bosh-lite/deployments` and `diego.yml` to be present in `~/workspace/diego-release/bosh-lite/deployments`. This assumes the analagous generate-manifest scripts have been run for those releases. If cf and diego manifests are in a different location then you may specify them as arguments:

        ./scripts/generate-bosh-lite-manifest <cf_deployment_manifest> <diego_deployment_manifest>

1. Finally, update your cf-release deployment to enable support for the Routing API, included in this release.
	
<<<<<<< 4347d078fe36f58eca526952abbfa505269805c9
		cd ~/workspace/cf-release

	Open ~/workspace/cf-release/bosh-lite/stubs/property_overrides.yml in an editor an add the `router.enable_routing_api:true` under `properties`. 
=======
	Open ~/workspace/cf-release/bosh-lite/stubs/property-overrides.yml in an editor an add the `router.enable_routing_api:true` under the root level `properties`.
>>>>>>> corrected manifest generation filename, `properties` ambiguity

		properties:
		  router:
		    enable_routing_api: true

	While you're at it, make your life easier by setting Diego as the default backend. TCP Routing is supported for applications on Diego only.
	
		properties:
		  cc:
		    default_to_diego_backend: true

	Then generate a new manifest for cf-release and re-deploy it.

		cd ~/workspace/cf-release
		./scripts/generate-bosh-lite-dev-manifest
		bosh -n deploy

## Deploying for High Availabilty

The TCP Router, TCP Emitter, and Routing API are stateless and horizontally scalable. Routing API depends on a clustered etcd data store. For high availability, deploy multiple instances of each job, distributed across regions of your infrastructure. 

### Configuring Your Load Balancer to Health Check TCP Routers

In order to determine whether TCP Router instances are eligible for routing requests to, configure your load balancer to periodically check the health of each instance by attempting a TCP connection. By default the health check port is 80. This port can be configured using the `haproxy.health_check_port` property in the property-overrides.yml stub file.

To simulate this health check manually on BOSH-lite:
  ```
  nc -z 10.244.8.2 80
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
See the README for [Router Acceptance Tests](https://github.com/cloudfoundry-incubator/cf-routing-acceptance-tests)

## Router API

For details refer to [Routing API](https://github.com/cloudfoundry-incubator/routing-api/blob/master/README.md).

## Testing the TCP Router Service manually

These instructions assume the release has been deployed to bosh-lite, along with cf-release and diego-release. See [Deploying cf-routing-release to a local BOSH-Lite instance](#deploying-tcp-router-to-a-local-bosh-lite-instance) above.

### Using CF 

The [lattice-app](https://github.com/cloudfoundry-samples/lattice-app) can be configured to listen on any port. The curl commands below will eventually be replaced with user-friendly commands in the [cf CLI](https://github.com/cloudfoundry/cli).

1. Push lattice app with no route, no start, and a custom start command that tells the app what ports to listen on
`$ cf p lattice -c "lattice-app --ports=7777,8888" --no-route --no-start`
- Use curl to change the app ports to the same the app will listen on. This will open these ports on the container.
`$ cf curl /v2/apps/4a10e0c9-0dd5-4d35-befe-619d28523504 -X PUT -d '{"ports":[7777,8888]}'`
- Start the app
`$ cf start lattice`
- As admin, create a shared-domain for the TCP router group
` $ cf create-shared-domain tcp.superman.cf-app.com --router-group default-tcp`
- Use curl to create a TCP route with a random port; the response includes the generated port.
` $ cf curl /v2/routes?generate_port=true -X POST -d '{"space_guid":"9723f7f2-e9ec-46dd-a4b9-26afed48f849","domain_guid":"6c2ea463-6c40-4c3e-ad14-74c6c9b3a529"}'`
- Use curl to map the route to the app, and specify the app port 7777
`$ cf curl /v2/route_mappings -X POST -d '{"route_guid":"e0a74cd6-b7de-4042-8da4-9dc45386b0e4","app_guid":"4a10e0c9-0dd5-4d35-befe-619d28523504","app_port":7777}'`
- curl the lattice app on the `/port` endpoint and it will return the local port it received the request on. Or you can add the domain you created above to your `/etc/hosts` file, resolving to the IP of the TCP Router.

	Ask BOSH what the IP of `tcp_router_z1` is

		bosh vms cf-warden-routing
	
	Now curl the app on that IP, with the port you received when creating the route, and the `/port` endpoint.
	 
		curl <ip of tcp router>:<port provided by CF>/port

### Using Diego API

With the [diego-release](https://github.com/cloudfoundry-incubator/diego-release) and this release deployed, use the  [Veritas](https://github.com/pivotal-cf-experimental/veritas) CLI to create an LRP. Follow the instructions on that projects README for creating an LRP (see the example JSON for Redis, which listens for TCP requests).

Once deployed, use Veritas to find out the host IP and port where the LRP is running:

```
$ veritas get-actual-lrp redis-1
```

Then test that TCP routing is functional with `nc` or the Redis CLI:

```
$ nc -v <host IP> <host port>

$ homebrew install redis
$ redis-cli -h <host IP> -p <host port> ping
```

### Using Routing API

1. Start the `tcp-sample-listener` on your local workstation
	```
	$ src/github.com/cloudfoundry-incubator/cf-routing-acceptance-tests/assets/tcp-sample-receiver/tcp-sample-receiver --address HOST:PORT
	```
	Substitute your workstation IP and a port of your choosing for `HOST:PORT` (e.g. 10.80.130.159:3333)

2. Using an API call, map an external port on the router to the host and port `tcp-sample-listener` is listening on. We would be POSTing to routing_api (listening on 10.24.0.134:3000 by default)

	```
	$ curl 10.24.0.134:3000/routing/v1/tcp_routes/create -X POST -d '[{"route":{"router_group_guid": "tcp-default", "port": 60000}, "backend_ip": "HOST", "backend_port": PORT}]'
	```
	Substitute the same workstation IP and a port you used in step #1 for `ip` and `port` (e.g. 10.80.130.159 and 3333). 
	
	A successful response will be an empty body with 200 status code.
	
3. You can then use netcat to send messages to the external port on the router, and verify they are received by `tcp-sample-listener`.
	```
	$ nc 10.244.8.2 60000
	isn't
	isn't
	this
	this
	cool?
	cool?
	```
	On the listener side, we see:
	```
	$  src/github.com/cloudfoundry-incubator/cf-routing-acceptance-tests/assets/tcp-sample-receiver/tcp-sample-receiver --address 10.80.130.159:3333
	Listening on 10.80.130.159:3333
	isn't
	this
	cool?
	```
	





