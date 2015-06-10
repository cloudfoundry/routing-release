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
    git clone https://github.com/GESoftware-CF/router-release.git
    cd router-release/

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

1. Checkout router-release (develop branch) from git

        cd ~/workspace
   		git clone https://github.com/GESoftware-CF/router-release.git
        cd ~/workspace/router-release/
	    git submodule init
	    git submodule update

1. Install spiff, a tool for generating BOSH manifests. spiff is required for
   running the scripts in later steps. The following installation method
   assumes that go is installed. For other ways of installing `spiff`, see
   [the spiff README](https://github.com/cloudfoundry-incubator/spiff).

        go get github.com/cloudfoundry-incubator/spiff

1. Generate and target router's manifest:

        cd ~/workspace/router-release
        ./scripts/generate-manifest \
            manifest-generation/bosh-lite-stubs \
            > ~/deployments/bosh-lite/router.yml
        bosh deployment ~/deployments/bosh-lite/router.yml

1. Do the BOSH Dance:

        bosh create release --force
        bosh -n upload release
        bosh -n deploy


## Running Acceptance tests

### Test setup

To run the Router Acceptance tests, you will need:
- a running router deployment
- an environment variable `ROUTER_API_CONFIG` which points to a `.json` file that contains the router api endpoint

The following commands will setup the `ROUTER_API_CONFIG` for a [bosh-lite](https://github.com/cloudfoundry/bosh-lite)
installation. Replace config as appropriate for your environment.


```bash
cd ~/workspace/router-release
cat > src/github.com/GESoftware-CF/cf-tcp-router-acceptance-tests/router_config.json <<EOF
{
  "address": "10.244.8.2",
  "port": 9999
}
EOF
```

### Running the tests

After correctly setting the `ROUTER_API_CONFIG` environment variable, the following command will run the tests:

```
./scripts/run-acceptance-tests
```

