# Cloud Foundry Routers [BOSH release]

----
This repo is a [BOSH](https://github.com/cloudfoundry/bosh) release for deploying TCP Router and associated tasks.

The TCP Router adds non-HTTP routing capabilities to Cloud Foundry.

----
## Developer Workflow

When working on individual components of TCP Router, work out of the submodules under `src/`.
See [Initial Setup](#initial-setup).

Run the individual component unit tests as you work on them using
[ginkgo](https://github.com/onsi/ginkgo).

---
##<a name="initial-setup"></a> Initial Setup

This BOSH release doubles as a `$GOPATH`. It will automatically be set up for
you if you have [direnv](http://direnv.net) installed.

    # fetch release repo
    mkdir -p ~/workspace
    cd ~/workspace
    git clone https://github.com/cloudfoundry-incubator/cf-routing-release.git
    cd cf-routing-release/

    # automate $GOPATH and $PATH setup
    direnv allow

    # initialize and sync submodules
    git submodule init
    git submodule update

If you do not wish to use direnv, you can simply `source` the `.envrc` file in the root
of the release repo.  You may manually need to update your `$GOPATH` and `$PATH` variables
as you switch in and out of the directory.

---
## Running Unit Tests

1. Install ginkgo

        go install github.com/onsi/ginkgo/ginkgo

2. Run unit tests

        ./scripts/run-unit-tests

---

## Deploying TCP Router to a local BOSH-Lite instance

1. Install and start [BOSH-Lite](https://github.com/cloudfoundry/bosh-lite),
   following its
   [README](https://github.com/cloudfoundry/bosh-lite/blob/master/README.md).

1. Download the latest Warden Trusty Go-Agent stemcell and upload it to BOSH-lite

        bosh public stemcells
        bosh download public stemcell (name)
        bosh upload stemcell (downloaded filename)

1. Checkout cf-routing-release (develop branch) from git

        cd ~/workspace
   		git clone https://github.com/cloudfoundry-incubator/cf-routing-release.git
        cd ~/workspace/cf-routing-release/
	    git submodule init
	    git submodule update

1. Install spiff, a tool for generating BOSH manifests. spiff is required for
   running the scripts in later steps. The following installation method
   assumes that go is installed. For other ways of installing `spiff`, see
   [the spiff README](https://github.com/cloudfoundry-incubator/spiff).

        go get github.com/cloudfoundry-incubator/spiff

1. Generate and target router's manifest:

        cd ~/workspace/cf-routing-release
        ./bosh-lite/make-manifest > ~/deployments/bosh-lite/router.yml
        bosh deployment ~/deployments/bosh-lite/router.yml

1. Do the BOSH Dance:

        bosh create release --force
        bosh -n upload release
        bosh -n deploy


## Running Acceptance tests

See the README for [Router Acceptance Tests](https://github.com/cloudfoundry-incubator/cf-tcp-router-acceptance-tests)

## Testing the TCP Router Service manually

These instructions assume the release has been deployed to bosh-lite

1. Start the `tcp-sample-listener` on your local workstation
	```
	$ src/github.com/cloudfoundry-incubator/cf-tcp-router-acceptance-tests/assets/tcp-sample-receiver/tcp-sample-receiver --address HOST:PORT
	```
	Substitute your workstation IP and a port of your choosing for `HOST:PORT` (e.g. 10.80.130.159:3333)

2. Using an API call, reserve an external port on the router and map it to the host and port `tcp-sample-listener` is listening on. By default, the API server listens on port 9999.

	```
	$ curl 10.244.8.2:9999/v0/external_ports -X POST -d '[{"backend_ip": "HOST", "backend_port":"PORT"}]'
	{"external_ip":"10.244.8.2","external_port":60000}
	```
	Substitute the same workstation IP and a port you used in step #1 for `backend_ip` and `backed_port` (e.g. 10.80.130.159 and 3333). 
	
	The response will include the external port on the router which is mapped to the `HOST` and `PORT` you provided.
	
3. You can then use netcat to send messages to the external port on the router, and verify they are received by `tcp-sample-listener`.
	```
	$ nc 10.244.8.2 50001
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


