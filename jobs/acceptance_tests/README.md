To run the acceptance tests as an errand, apply the following ops-file to your `cf-deployment`:

```yaml
---
- type: replace
  path: /instance_groups/name=routing_acceptance_tests?
  value:
    name: routing_acceptance_tests
    lifecycle: errand
    azs:
    - z1
    instances: 1
    vm_type: minimal
    stemcell: default
    networks:
    - name: default
    jobs:
    - name: acceptance_tests
    release: routing
    properties:
      tcp_emitter:
        oauth_secret: "((uaa_clients_tcp_emitter_secret))"
      acceptance_tests:
        nodes: 1
        addresses: [10.244.0.137] # replace this with the IP address of your TCP router
        cloud_controller:
          api: "api.((system_domain))"
          admin_user: "admin"
          admin_password: "((cf_admin_password))"
          apps_domain: "((system_domain))"
          use_http: true
        system_domain: "((system_domain))"
        skip_ssl_validation: true
        include_http_routes: true
```
