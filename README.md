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

#### DesiredLRPCreateRequest
In order to receive TCP traffic on a given application port, the [DesiredLRPCreateRequest] (https://github.com/cloudfoundry-incubator/receptor/blob/master/doc/lrps.md#describing-desiredlrps) should be created as follows:

```
{
    "process_guid": "some-guid",
    "domain": "some-domain",

    "instances": 17,

    "rootfs": "VALID-ROOTFS",

    "setup": ACTION,
    "action":  ACTION,
    "monitor": ACTION,

    "ports": [8080, 5050, 5222],
    "routes": {
        "cf-router": [
            {
                "hostnames": ["a.example.com", "b.example.com"],
                "port": 8080
            }, {
                "hostnames": ["c.example.com"],
                "port": 5050
            }
        ],
        "tcp-router" : [
            {
                "external_port":60000,
                "container_port":5222
            }
        ]
    }
}
```

Let’s break this down:

1. The `ports` section now includes the container port on which the application will receive TCP traffic.

1. The `tcp-router` section within `routes` includes the association (*mapping*) between the external port on the TCP Router and the corresponding container port.

#### DesiredLRPUpdateRequest
In order to update an existing Desired LRP with new external port mapping, the [DesiredLRPUpdateRequest](https://github.com/cloudfoundry-incubator/receptor/blob/master/doc/lrps.md#updating-desiredlrps) should be created like this:

```
{
    "instances": 17,
    "routes": {
        "cf-router": [
            {
                "hostnames": ["a.example.com", "b.example.com"],
                "port": 8080
            }, {
                "hostnames": ["c.example.com"],
                "port": 5050
            }
        ],
        "tcp-router" : [
            {
                "external_port":60000,
                "container_port":5222
            }
        ]
    },
    "annotation": "arbitrary metadata"
}
```

Let’s break this down:

1. The `container_port` must have been already specified as part of the `ports` section in the DesiredLRPCreateRequest

1. The `tcp-router` section within `routes` includes the new association (*mapping*) between the external port on the TCP Router and the corresponding container port.


