# routing ci

Public configuration for the CF Routing team's CI pipelines

[CI Dashboard: `dashboard.routing.cf-app.com`](http://dashboard.routing.cf-app.com)

### Dashboard config

#### DNS
 - `cf-app.com` base domain includes an NS record that delegates `routing.cf-app.com` to [DNS Zone `routing-team`](https://console.cloud.google.com/net-services/dns/zones/routing-team?project=cf-routing)
 - CNAME record `axxxxxxxl.routing.cf-app.com` is required for domain verification.  If you remove it, everything breaks!
 - Then `dashboard.routing.cf-app.com` is a CNAME for `c.storage.googleapis.com` in order to support [GCP Static Website Hosting](https://cloud.google.com/storage/docs/hosting-static-website)

## Helper Scripts

There are a handful of helper scripts and functions in the `/scripts` directory. To use them, add the directory to your path and source the directory:

For `cf_login` and `bosh_login` to environments:
```bash
source ~/workspace/routing-release-ci/ci/scripts/script_helpers.sh
cf_login <env_name>
```

For local bosh-lite management:
```bash
export PATH=$PATH:$(pwd)/scripts
local_bosh_lite_create
```
