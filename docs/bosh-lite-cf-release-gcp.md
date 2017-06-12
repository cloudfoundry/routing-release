# Deploying cf-release on BOSH-lite on GCP

The following instructions provided detailed steps to deploy a [cf-release](https://github.com/cloudfoundry/cf-release) deployment with TCP routing and optional advanced steps to enable Isolation Segments. These instructions particularly were created to use a BOSH-lite on Google Cloud Platform.

**Note**: The below instructions use bosh v2 cli.

## Overview of Steps Required

1. [Setup GCP Environment for BOSH-lite director](#setup-gcp-environment-for-bosh-lite-director)
1. [Setup GCP Environment for deployments](#setup-gcp-environment-for-deployments)
1. [Prepare stubs for cf-release](#prepare-stubs-for-cf-release)
1. [Generate deployment manifests and deploy](#generate-deployment-manifests-and-deploy)
1. *Optional*: [Routing Isolation Segments](#routing-isolation-segments)

## Instructions
### Setup GCP Environment for BOSH-lite director
1. Run the following commands to generate the `service-account.key.json`
```
export ACCOUNT_NAME="YOUR_ACCOUNT_NAME"
export PROJECT_ID="GCP_PROJECT_ID"
$ gcloud auth login
$ gcloud config set project ${PROJECT_ID}
$ gcloud iam service-accounts create ${ACCOUNT_NAME}
$ gcloud iam service-accounts keys create "service-account.key.json" --iam-account "${ACCOUNT_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"
$ gcloud projects add-iam-policy-binding ${PROJECT_ID} --member "serviceAccount:${ACCOUNT_NAME}@${PROJECT_ID}.iam.gserviceaccount.com" --role 'roles/editor'
```
1. Follow steps 1 and 2 in this documentation to pave a GCP project and deploy the BOSH-lite director: https://github.com/cloudfoundry/cf-deployment/blob/master/bosh-lite-on-gcp-deployment-guide.md
1. Set up DNS to point to the nameservers of the new DNS zone created by Terraform using the `shared-dns` account on AWS.

### Setup GCP Environment for deployments
1. Generate certificates:
```
openssl req -x509 -newkey rsa:2048 -keyout key.pem -out cert.pem -nodes
```

#### Create Instance Group
1. Navigate to GCP > Compute Engine > Instance groups > Create Instance Group
1. Create an instance group with the following properties:
   - Name: `PROJECT_NAME`
   - Zone: `AZ_OF_BOSH_LITE`
   - Group type:  `Unmanaged instance group`
   - Network: `PROJECT_NAME-network`
   - VM instances: `vm-xxxx` (only one instance to choose from, BOSH-lite director)

#### Create health checks
1. Navigate to GCP > Compute Engine > Health checks > Create Health Check.
1. Create health checks with the following properties:

| Name | Path | Protocol | Port |
| ---- | ---- | ---- | ---- |
| PROJECT_NAME-http-hc | /health | HTTP | 8080 |
| PROJECT_NAME-tcp-hc | /health | HTTP | 8082 |

#### Create Firewall Rules
1. Navigate to GCP > Networking > Firewall Rules
1. Edit PROJECT_NAME-firewall to add the following ports:  `81,1024-1123, 8080-8082`

#### Create Load balancers
##### HTTP Load Balancer
1. Navigate to GCP > Networking > Load Balancing > Create Load Balancer
1. Create HTTP(S) Load Balancing with Name: `PROJECT_NAME-http`
  1. Backend Configuration > Backend Service > Create backend service:
      - Name:  `shared-PROJECT_NAME-router`
      - Edit Named port: `http`
      - New backend:
        - Instance-group:  `PROJECT_NAME`
        - Port numbers: `80`
      - Health check:  `PROJECT_NAME-http-hc`
  1. Host and path rules. Leave Host and Path as defaults and set Backends to `shared-PROJECT_NAME-router`
  1. Frontend configuration:
      - HTTP Frontend
        - You can leave frontend Name blank.
        - Protocol:  `HTTP`
        - Port: `80`
        - IP address:  Create IP address
          - Name:  `PROJECT_NAME-http-lb-ip`
      - HTTPS Frontend
        - You can leave frontend Name blank.
        - Protocol:  `HTTPS`
        - Port: `443`
        - IP address: `PROJECT_NAME-http-lb-ip`
        - Certificate: Create a new certificate
          - Name:  `PROJECT_NAME-cert`
          - Public key certificate:  upload cert.pem
          - Private key:  upload key.pem
1. Click Create to complete Load Balancer configuration

##### TCP Load Balancer
1. Create TCP Load Balancing with the following properties:
    - Internet facing or internal only:  `From Internet to my VMs`
    - Multiple regions or single region:  `single region only`
    - Name:  `PROJECT_NAME-tcp`
  1. Backend configuration:
      - Region: `PROJECT_REGION`
      - Select Existing instances: `Add an instance` (only 1 vm present)
      - Health check:  `PROJECT_NAME-tcp-hc`
  1. Frontend configuration
      - IP:  `Create IP address`
        - Name:  `PROJECT_NAME-tcp-lb-ip`
      - Port:  `1024-1123`

##### Websocket Load Balancer
1. Create TCP Load Balancing with the following properties:
    - Internet facing or internal only:  `From Internet to my VMs`
    - Multiple regions or single region:  `single region only`
    - Name:  `PROJECT_NAME-ws`
  1. Backend configuration:
      - Region:  `PROJECT_REGION`
      - Select Existing instances: `Add an instance` (only 1 vm present)
      - Health check:  `PROJECT_NAME-http-hc`
  1. Frontend configuration
      - Add Frontend IP and port:
        - IP:  `Create IP address`
          - Name:  `PROJECT_NAME-ws-lb-ip`
        - Port:  `80`
      - Add Frontend IP and port:
        - IP:  `PROJECT_NAME-ws-lb-ip`
        - Port:  `443`
      - Add Frontend IP and port
        - IP:  `PROJECT_NAME-ws-lb-ip`
        - Port:  `4443`

### Setup Cloud DNS
1. GCP > Networking > Cloud DNS
    - Point `*.PROJECT_NAME.cf-app.com` to the HTTP load balancer IP address
    - Point `tcp.PROJECT_NAME.cf-app.com` to the TCP load balancer IP address

### Prepare stubs for cf-release
- Create a directory to hold the stubs
```
mkdir -p stubs/{cf,routing}
```
- Create each of the below stubs with your properties:
`cf/admin_password.yml`:
```
---
properties:
  uaa:
    scim:
      users:
      - name: CF_USER
        password: CF_ADMIN_PASSWORD
        firstname:
        lastname:
        groups:
        - scim.write
        - scim.read
        - openid
        - cloud_controller.admin
        - doppler.firehose
        - routing.router_groups.read
        - routing.router_groups.write
    admin:
      client_secret: CLIENT_SECRET
  acceptance_tests:
    admin_user: CF_USER
    admin_password: CF_ADMIN_PASSWORD
    nodes: 3
    include_sso: true
    include_operator: true
    include_logging: true
    include_security_groups: true
    include_internet_dependent: true
    include_services: true
  smoke_tests:
    user: admin
    password: CF_ADMIN_PASSWORD
```
`cf/bosh-lite.yml`:
```
resource_pools:
- name: router_z1
  cloud_properties:
    ports:
    - host: 80
    - host: 8080
    - host: 443
    - host: 4443
      container: 443
    - host: 2222
- name: router_z2
  cloud_properties:
    ports:
    - host: 81
      container: 80
    - host: 8081
      container: 8080
    - host: 444
      container: 443
    - host: 4444
      container: 443
    - host: 2223
```
`cf/domain.yml`:
```
properties:
  system_domain: YOUR_DOMAIN
```
`cf/doppler.yml`:
```
properties:
  doppler:
    port: 443
```
`cf/router-ssl.yml`:
```
properties:
  router:
    enable_ssl: true
    ssl_cert: SSL_CERT
    ssl_key: SSL_KEY
```
`routing/iaas-settings.yml`:
```
iaas_settings:
  stemcell:
    name: bosh-warden-boshlite-ubuntu-trusty-go_agent
    version: latest
  subnet_configs:
    - name: router1
      type: manual
      subnets:
      - cloud_properties: {}
        range: 10.244.14.0/30
        reserved:
        - 10.244.14.1
        static:
        - 10.244.14.2
      - cloud_properties: {}
        range: 10.244.14.4/30
        reserved:
        - 10.244.14.5
        static:
        - 10.244.14.6
      - cloud_properties: {}
        range: 10.244.14.8/30
        reserved:
        - 10.244.14.9
        static:
        - 10.244.14.10
      - cloud_properties: {}
        range: 10.244.14.12/30
        reserved:
        - 10.244.14.13
        static: []
      - cloud_properties: {}
        range: 10.244.14.16/30
        reserved:
        - 10.244.14.17
        static: []
      - cloud_properties: {}
        range: 10.244.14.20/30
        reserved:
        - 10.244.14.21
        static: []
      - cloud_properties: {}
        range: 10.244.14.24/30
        reserved:
        - 10.244.14.25
        static: []
      - cloud_properties: {}
        range: 10.244.14.28/30
        reserved:
        - 10.244.14.29
        static: []
      - cloud_properties: {}
        range: 10.244.14.32/30
        reserved:
        - 10.244.14.33
        static: []
      - cloud_properties: {}
        range: 10.244.14.36/30
        reserved:
        - 10.244.14.37
        static: []
    - name: router2
      type: manual
      subnets:
      - cloud_properties: {}
        range: 10.244.10.0/30
        reserved:
        - 10.244.10.1
        static:
        - 10.244.10.2
      - cloud_properties: {}
        range: 10.244.10.4/30
        reserved:
        - 10.244.10.5
        static:
        - 10.244.10.6
      - cloud_properties: {}
        range: 10.244.10.8/30
        reserved:
        - 10.244.10.9
        static:
        - 10.244.10.10
      - cloud_properties: {}
        range: 10.244.10.12/30
        reserved:
        - 10.244.10.13
        static: []
      - cloud_properties: {}
        range: 10.244.10.16/30
        reserved:
        - 10.244.10.17
        static: []
      - cloud_properties: {}
        range: 10.244.10.20/30
        reserved:
        - 10.244.10.21
        static: []
      - cloud_properties: {}
        range: 10.244.10.24/30
        reserved:
        - 10.244.10.25
        static: []
    - name: router3
      type: manual
      subnets:
      - cloud_properties: {}
        range: 10.244.12.0/30
        reserved:
        - 10.244.12.1
        static:
        - 10.244.12.2
      - cloud_properties: {}
        range: 10.244.12.4/30
        reserved:
        - 10.244.12.5
        static:
        - 10.244.12.6
      - cloud_properties: {}
        range: 10.244.12.8/30
        reserved:
        - 10.244.12.9
        static:
        - 10.244.12.10
      - cloud_properties: {}
        range: 10.244.12.12/30
        reserved:
        - 10.244.12.13
        static: []
      - cloud_properties: {}
        range: 10.244.12.16/30
        reserved:
        - 10.244.12.17
        static: []
      - cloud_properties: {}
        range: 10.244.12.20/30
        reserved:
        - 10.244.12.21
        static: []
      - cloud_properties: {}
        range: 10.244.12.24/30
        reserved:
        - 10.244.12.25
        static: []

  compilation_cloud_properties: {}
  resource_pool_cloud_properties:
    - name: routing_api_z1
      cloud_properties: {}
    - name: routing_api_z2
      cloud_properties: {}
    - name: routing_api_z3
      cloud_properties: {}
    - name: tcp_router_z1
      cloud_properties:
        ports:
        - host: 1024-1123
        - host: 8082
          container: 80
    - name: tcp_router_z2
      cloud_properties: {}
    - name: tcp_router_z3
      cloud_properties: {}
    - name: tcp_emitter_z1
      cloud_properties: {}
    - name: tcp_emitter_z2
      cloud_properties: {}
    - name: tcp_emitter_z3
      cloud_properties: {}
    - name: small_errand
      cloud_properties: {}
```

### Generate deployment manifests and deploy
#### cf-release
Generate the cf-release deployment manifest:
```
CF_RELASE_DIR/scripts/generate_deployment_manifest bosh-lite STUBS_DIR/* > cf.yml

bosh -e PROJECT_NAME upload-release https://bosh.io/d/github.com/cloudfoundry/cf-release
bosh -e PROJECT_NAME upload-stemcell https://bosh.io/d/stemcells/bosh-warden-boshlite-ubuntu-trusty-go_agent

bosh -e PROJECT_NAME -d cf-warden deploy cf.yml
```

#### diego-release
Generate the diego deployment manifest:
```
DIEGO_RELEASE_DIR/scripts/generate-deployment-manifest \
  -c cf.yml \
  -i DIEGO_RELEASE_DIR/manifest-generation/bosh-lite-stubs/iaas-settings.yml \
  -p DIEGO_RELEASE_DIR/manifest-generation/bosh-lite-stubs/property-overrides.yml \
  -n DIEGO_RELEASE_DIR/manifest-generation/bosh-lite-stubs/instance-count-overrides.yml \
  -x \
  -s DIEGO_RELEASE_DIR/manifest-generation/bosh-lite-stubs/postgres/diego-sql.yml \
  -v DIEGO_RELEASE_DIR/manifest-generation/bosh-lite-stubs/release-versions.yml \
  > diego.yml

bosh -e PROJECT_NAME upload-release https://bosh.io/d/github.com/cloudfoundry/diego-release
bosh -e PROJECT_NAME upload-release https://bosh.io/d/github.com/cloudfoundry/cflinuxfs2-release
bosh -e PROJECT_NAME upload-release https://bosh.io/d/github.com/cloudfoundry/garden-runc-release

bosh -e PROJECT_NAME -d cf-warden-diego deploy diego.yml
```

#### routing-release
Generate the routing deployment manifest:
```
DIRECTOR_UUID=DIRECTOR_UUID \
  ${ROUTING_RELEASE_DIR}/scripts/generate-manifest \
  -c cf.yml \
  -d diego.yml \
  -l ${ROUTING_RELEASE_DIR}/bosh-lite/stubs/property-overrides.yml \
  -l ${ROUTING_RELEASE_DIR}/bosh-lite/stubs/instance-count-overrides.yml \
  -l ${ROUTING_RELEASE_DIR}/bosh-lite/stubs/persistent-disk-overrides.yml \
  -l stubs/routing/iaas-settings.yml \
  > routing.yml

bosh -e PROJECT_NAME upload-release https://bosh.io/d/github.com/cloudfoundry-incubator/cf-routing-release

bosh -e PROJECT_NAME -d cf-warden-routing deploy routing.yml
```

#### Update cf-release
```
spiff merge cf.yml ROUTING_RELEASE_DIR/bosh-lite/stubs/cf/* > updated-cf.yml

bosh -e PROJECT_NAME -d cf-warden deploy updated-cf.yml
```

#### Validate TCP routing with smoke tests
`bosh -e PROJECT_NAME -d cf-warden-routing run-errand routing_smoke_tests`


### Routing Isolation Segments
#### Make changes to GCP environment
##### Create health checks
1. Navigate to GCP > Compute Engine > Health checks > Create Health Check.
1. Create health checks with the following properties:

| Name | Path | Protocol | Port |
| ---- | ---- | ---- | ---- |
| PROJECT_NAME-http-is1-hc | /health | HTTP | 8081 |

##### Create Load balancer
1. Navigate to GCP > Networking > Load Balancing > Create Load Balancer
1. Create HTTP(S) Load Balancing with the following properties with Name:  `PROJECT_NAME-is1-http`
  1. Backend Configuration > Backend Service > Create backend service:
      - Name:  `is1-PROJECT_NAME-router`
      - Edit Named port: `http-iso`
      - New backend:
        - Instance-group:  PROJECT_NAME
        - Port numbers: 81
      - Health check:  PROJECT_NAME-http-is1-hc
  1. Host and path rules. Leave Host and Path as defaults and set Backends to `is1-PROJECT_NAME-router`
  1. Frontend configuration:
      - HTTP Frontend
        - You can leave frontend Name blank.
        - Protocol:  `HTTP`
        - Port: `80`
        - IP address:  Create IP address
          - Name:  `PROJECT_NAME-http-is1-lb-ip`
      - HTTPS Frontend
        - You can leave frontend Name blank.
        - Protocol:  `HTTPS`
        - Port: `443`
        - IP address: `PROJECT_NAME-http-is1-lb-ip`
        - Certificate: `PROJECT_NAME-cert`
1. Click Create to complete Load Balancer configuration

##### Update Cloud DNS
1. GCP > Networking > Cloud DNS
1. Point `*.is1.PROJECT_NAME.cf-app.com` to the HTTP IS1 load balancer IP address

#### Stub changes
1. Create `stubs/cf/router-is.yml`:
```
---
jobs:
- name: router_z2
  instances: 1
  properties:
    router:
      isolation_segments: [is1]
      routing_table_sharding_mode: segments
- name: ha_proxy_z1
  instances: 0
```

#### Generate deployment manifests, make manual changes, and deploy

##### cf-release
1. Run `CF_RELASE_DIR/scripts/generate_deployment_manifest bosh-lite STUBS_DIR/* > cf.yml`
1. In `cf.yml`:
	- update `router_z1` properties to `router.routing_stable_sharding_mode: shared-and-segments`
	- update `properties.cc.diego.temporary_local_apps: true`
1. `bosh -e PROJECT_NAME -d cf-warden deploy cf.yml`

##### diego-release
1. Update `diego.yml` to include IS by editing the `cell_z2` instance group:
```
  name: cell_z2
  properties:
    diego:
      rep:
        placement_tags: [is1]
```
2. `bosh -e PROJECT_NAME -d cf-warden-diego deploy diego.yml`

##### Validate Isolation Segment
```
# create org, space, iso-seg, and enable appropriately
cf create-org isolated
cf target -o isolated
cf create-space iso-space
cf target -o "isolated" -s "iso-space"
cf create-isolation-segment is1
cf enable-org-isolation isolated is1
cf set-space-isolation-segment iso-space is1

# create domain for is1
cf create-domain isolated is1.PROJECT_NAME.cf-app.com

# from a test asset dir, we're using golang
cf push golang -d is1.PROJECT_NAME.cf-app.com

# the following curl should succeed
curl golang.is1.PROJECT_NAME.cf-app.com
```

The following command should return a `404 Not Found`. This validates that an app in the isolation segment cannot be reached via the shared router.
```
curl -H "Host: golang.is1.PROJECT_NAME.cf-app.com" blah.PROJECT_NAME.cf-app.com
```
