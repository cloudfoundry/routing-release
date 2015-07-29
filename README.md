# Cloud Foundry Routers [BOSH release]

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

## Deploying TCP Router to a local BOSH-Lite instance

1. Install and start [BOSH-Lite](https://github.com/cloudfoundry/bosh-lite),
   following its
   [README](https://github.com/cloudfoundry/bosh-lite/blob/master/README.md).

1. Download the latest Warden Trusty Go-Agent stemcell and upload it to BOSH-lite

	```
	curl -L -J -O https://bosh.io/d/stemcells/bosh-warden-boshlite-ubuntu-trusty-go_agent
	bosh upload stemcell bosh-warden-boshlite-ubuntu-trusty-go_agent
	```
	
1. Clone the repo and sync submodules

   See [Get the code](#get-the-code)

1. Install spiff, a tool for generating BOSH manifests. spiff is required for
   running the scripts in later steps. The following installation method
   assumes that go is installed. For other ways of installing `spiff`, see
   [the spiff README](https://github.com/cloudfoundry-incubator/spiff).

        go get github.com/cloudfoundry-incubator/spiff

1. Generate and target router's manifest:

        cd ~/workspace/cf-routing-release
        ./bosh-lite/make-manifest <cf_deployment_manifest>

1. Create and upload cf-routing release, either by using a final release or creating your own development release as described below:

    * Upload the final available release

            cd ~/workspace/cf-routing-release
            bosh -n upload release releases/cf-routing-<lastest_version>.yml
            bosh -n deploy

    * Or create and upload your release

            bosh create release --force
            bosh -n upload release
            bosh -n deploy

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
See the README for [Router Acceptance Tests](https://github.com/cloudfoundry-incubator/cf-tcp-router-acceptance-tests)

## Router API

For details refer to [TCP Router API] (https://github.com/cloudfoundry-incubator/cf-tcp-router/blob/master/overview.md).

## Testing the TCP Router Service manually

These instructions assume the release has been deployed to bosh-lite

### Using Routing API

1. Start the `tcp-sample-listener` on your local workstation
	```
	$ src/github.com/cloudfoundry-incubator/cf-tcp-router-acceptance-tests/assets/tcp-sample-receiver/tcp-sample-receiver --address HOST:PORT
	```
	Substitute your workstation IP and a port of your choosing for `HOST:PORT` (e.g. 10.80.130.159:3333)

2. Using an API call, map an external port on the router to the host and port `tcp-sample-listener` is listening on. By default, the API server listens on port 9999.

	```
	$ curl 10.244.8.2:9999/v0/external_ports -X POST -d '[{"external_port":60000, "backends": [{"ip": "HOST", "port":"PORT"}]}]'	
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
	$  src/github.com/cloudfoundry-incubator/cf-tcp-router-acceptance-tests/assets/tcp-sample-receiver/tcp-sample-receiver --address 10.80.130.159:3333
	Listening on 10.80.130.159:3333
	isn't
	this
	cool?
	```
	
### Using Diego API

The following tutorial starts [the Redis Docker image](https://registry.hub.docker.com/_/redis/) as an LRP, and automatically configures the TCP Router to route traffic for a requested external port on the router to the Redis process. We will be using the Diego Receptor API; for more information see [Diego API Docs](https://github.com/cloudfoundry-incubator/receptor/tree/master/doc). 

#### Prerequisites

- [bosh-lite](https://github.com/cloudfoundry/bosh-lite)
- [cf-release](https://github.com/cloudfoundry/cf-release) deployment - must be deployed with configuration for Diego, see [diego-release README](https://github.com/cloudfoundry-incubator/diego-release)
- [diego-release deployment](https://github.com/cloudfoundry-incubator/diego-release) - See README for deployment instructions 
  - **Important** Diego must be deployed with manifest property `properties.diego.executor.allow_privileged: true`. This is required because the Redis process will be started with user root.
- cf-routing-release deployment - this release

This example was tested with [diego-release 0.1369.0](https://github.com/cloudfoundry-incubator/diego-release/releases/tag/0.1369.0) and [cf-release](https://github.com/cloudfoundry/cf-release) sha 07576287. Compatible versions of diego-release and cf-release are documented [here](https://github.com/cloudfoundry-incubator/diego-cf-compatibility/blob/master/compatibility-v1.csv).

1. Create a domain for your testing. Domains are namespaces for LRPs in Diego and are not to be confused with domains in Cloud Foundry.
	```
	$ curl receptor.10.244.0.34.xip.io/v1/domains/redis-example -X PUT
	
	$ curl receptor.10.244.0.34.xip.io/v1/domains
	["redis-example","cf-apps"]
	```
	
2. Create the desiredLRP

	In order for TCP traffic to be routed to a container port, the [DesiredLRPCreateRequest] (https://github.com/cloudfoundry-incubator/receptor/blob/master/doc/lrps.md#describing-desiredlrps) must include an `external_port`, along with `container_port` set to one of the values of `ports`. TCP Router uses the `container_port` to identify the discover the `host_port` provided by Diego once the actualLRP is created.

	```
	$ curl receptor.10.244.0.34.xip.io/v1/desired_lrps -X POST -d '{"process_guid":"92bcf571-630f-4ad3-bfa6-146afd40bded","domain":"redis-example","rootfs":"docker:///redis","instances":1,"ports":[6379],"action":{"run":{"path":"/entrypoint.sh","args":["redis-server"],"dir":"/data","user":"root"}},"routes":{"tcp-router":[{"external_port":50000,"container_port":6379}]}}'
	```
	
	Let's take a closer look at the body of this request:
	```
	{
	    "process_guid":"92bcf571-630f-4ad3-bfa6-146afd40bded",
	    "domain":"redis-example",
	    "rootfs":"docker:///redis",
	    "instances":1,
	    "ports":[
	        6379
	    ],
	    "action":{
	        "run":{
	            "path":"/entrypoint.sh",
	            "args":[
	                "redis-server"
	            ],
	            "dir":"/data",
	            "user":"root"
	        }
	    },
	    "routes":{
	        "tcp-router":[
	            {
	                "external_port":50000,
	                "container_port":6379
	            }
	        ]
	    }
	}
	```

	- `ports` declares the container ports on which the LRP is listening and for which Diego will create a `host_port`. It is to the `host_port` that TCP Router will will route TCP requests.
	- The contents of `routes` are opaque to Diego, and provides a mechanism for Diego API clients to pass through configuration to the routing tier which is [listening for events from Diego](https://github.com/cloudfoundry-incubator/receptor/blob/master/doc/api_lrps.md#receiving-events-when-actual-or-desired-lrps-change).
	- Within `routes`, `tcp-router` is used to declare the `external_port` on which TCP Router will listen for requests to this LRP, and the `container_port` which enables TCP Router to discover the `host_port` once the actualLRP is created. 

	Within a few moments, Diego will generate the actualLRP:
	```
	$ curl receptor.10.244.0.34.xip.io/v1/actual_lrps/92bcf571-630f-4ad3-bfa6-146afd40bded | jq .
	[
	  {
	    "process_guid": "92bcf571-630f-4ad3-bfa6-146afd40bded",
	    "instance_guid": "3f10ebc6-ee79-4da7-6b6d-d8a9bad3e145",
	    "cell_id": "cell_z1-0",
	    "domain": "redis-example",
	    "index": 0,
	    "address": "10.244.16.138",
	    "ports": [
	      {
	        "container_port": 6379,
	        "host_port": 60005
	      }
	    ],
	    "state": "RUNNING",
	    "crash_count": 0,
	    "since": 1437158962666436000,
	    "evacuating": false,
	    "modification_tag": {
	      "epoch": "f72c4043-a9f6-4ca9-7ffe-800cf2ed3137",
	      "index": 2
	    }
	  }
	]
	```
	
	Notice `address` and `host_port`, this is the IP and port to which TCP Router route traffic for the requested `external_port`. Diego will handle routing from `host_port` to `container_port`.

3. Test that a request to the `external_port` is received by the Redis process
	```
	$ bosh vms cf-warden-routing
	Deployment `cf-warden-routing'
	
	Director task 20
	
	Task 20 done
	
	+-----------------+---------+---------------+------------+
	| Job/index       | State   | Resource Pool | IPs        |
	+-----------------+---------+---------------+------------+
	| tcp_router_z1/0 | running | tcp_router_z1 | 10.244.8.2 |
	+-----------------+---------+---------------+------------+
	
	VMs total: 1
	
	$ redis-cli -h 10.244.8.2 -p 50000 ping
	PONG
	```

4. (Optional) Add an External Port

	You can add an external port for which TCP Router will route traffic to the LRP, however the ports opened on the container cannot be modified (a new LRP must be created). The `container_port` provided with the update request must have been included in the `ports` field with the createLRP request. We'll use [DesiredLRPUpdateRequest](https://github.com/cloudfoundry-incubator/receptor/blob/master/doc/api_lrps.md#modifying-desiredlrps) and include the additional `external_port` and the same `container_port` we did with the createLRP request earlier.
	
	**Note** The header `-H 'Content-Type: application/json'` is required for this to work.
	```
	$ curl receptor.10.244.0.34.xip.io/v1/desired_lrps/92bcf571-630f-4ad3-bfa6-146afd40bded -X PUT -d '{"routes":{"tcp-router":[{"external_port":50001,"container_port":6379}]}}' -H 'Content-Type: application/json'
	
	$ redis-cli -h 10.244.8.2 -p 50001 ping
	PONG
	```
	

