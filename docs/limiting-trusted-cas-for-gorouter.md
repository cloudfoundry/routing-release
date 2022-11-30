# Limiting Trusted CAs for Gorouter

This doc is for operators who want to use the new "only trust client CA certs" feature for gorouter to limit the CA certs that gorouter trusts. 

## Version
This feature is available in [0.210.0](https://github.com/cloudfoundry/routing-release/releases/tag/0.210.0) 

## Context
Operators already had the ability to add custom CAs to the gorouter using `router.ca_certs`, but they didn't have the ability to stop the gorouter from trusting the default CAs that are provided with the stemcell.

## Feature Description 

We have added two new bosh properites: 

* `router.client_ca_certs` (`optional; default: ""`) that will allow the operators to specify CA certs for the gorouter to trust for client requests.
* `router.only_trust_client_ca_certs` (`default: false`), that will allow the operator to decide if gorouter should _only_ trust the above CA certs, or concatenate them with those in `router.ca_certs` and those provided by the stemcell
  * When `true`, only the certs configured in `router.client_ca_certs` are loaded as trusted client certs
  * When `false`, all the certs in `router.ca_certs`, `router.client_ca_certs`, plus the local system store are trusted client certificates.  **This maintains backward compatibility.**
  
## Example Usage

These examples assume that the load balancer is not terminating TLS. 

### Scenario 1: `only_trust_client_ca_certs: false`

With `only_trust_client_ca_certs: false`, all the certs in `router.ca_certs`, `router.client_ca_certs`, plus the local system store are trusted client certificates. This is the backwards compatible option.

```
router:
  ca_certs:
    - a-cert-named-apple
  client_ca_certs: |
   a-cert-named-cucumber
  only_trust_client_ca_certs: false
  client_cert_validation: require
```

```
# Using cert in ca_certs
curl --cert apple.crt --key apple.key https://GOROUTER_IP -H "HOST: dora.example.com/
# OK

# Using cert in client_ca_certs
curl --cert cucumber.crt --key cucumber.key https://dora.example.com/
# OK

# Using cert in the local store
curl --cert some-stemcell-trusted-cert.crt --key some-stemcell-trusted-cert.key https://dora.example.com/
# OK

# Using other cert
curl --cert melon.crt --key melon.key https://dora.example.com/
# FAIL
```

### Scenario 2: `only_trust_client_ca_certs: true`

With `only_trust_client_ca_certs: true`, _only_ the certs configured in `router.client_ca_certs` are loaded as trusted client certs.

```
router:
  ca_certs:
   - a-cert-named-apple
  client_ca_certs: |
   a-cert-named-cucumber
  only_trust_client_ca_certs: true
  client_cert_validation: require
```

```
# Using cert in ca_certs
curl --cert apple.crt --key apple.key https://dora.example.com/
# FAIL

# Using cert in client_ca_certs
curl --cert cucumber.crt --key cucumber.key https://dora.example.com/
# OK

# Using cert in the local store
curl --cert some-stemcell-trusted-cert.crt --key some-stemcell-trusted-cert.key https://dora.example.com/
# FAIL

# Using other cert
curl --cert melon.crt --key melon.key https://dora.example.com/
# FAIL
```


