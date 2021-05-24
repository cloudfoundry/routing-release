[![Build Status](https://travis-ci.org/cloudfoundry-incubator/uaa-go-client.svg?branch=master)](https://travis-ci.org/cloudfoundry-incubator/uaa-go-client)

# NOTICE

**This library is no longer maintained.**

Consider using the [`cloudfoundry-community/go-uaa`](https://github.com/cloudfoundry-community/go-uaa) client library instead.

# uaa-go-client
A go library for Cloud Foundry [UAA](https://github.com/cloudfoundry/uaa) that provides the following:
- fetch access tokens (including ability to cache tokens)
- decode tokens
- get token signing key


## Setup

As dependecies for uaa-go-client are not vendored, you should clone the [routing-release](https://github.com/cloudfoundry-incubator/routing-release) repo to get compatible versions of its dependencies.
```bash
git clone https://github.com/cloudfoundry-incubator/routing-release
cd routing-release
./scripts/update
cd src/code.cloudfoundry.org/uaa-go-client
```

If you are using this client as a dependency in your own go project, import it from `code.cloudfoundry.org/uaa-go-client`, then determine compatible versions of this projects dependencies by cloning [routing-release](https://github.com/cloudfoundry-incubator/routing-release).

## Example
This example client connects to UAA using https and skips cert verification.
```go
cfg := &config.Config{
  ClientName:       "client-name",
	ClientSecret:     "client-secret",
	UaaEndpoint:      "https://uaa.service.cf.internal:8443",
	SkipVerification: true,
}

uaaClient, err = client.NewClient(logger, cfg, clock)
if err != nil {
  log.Fatal(err)
  os.Exit(1)
}

fmt.Printf("Connecting to: %s ...\n", cfg.UaaEndpoint)

token, err = uaaClient.FetchToken(true)
if err != nil {
  log.Fatal(err)
  os.Exit(1)
}

fmt.Printf("Token: %#v\n", token)
```

## Example command line clients
The following example clients can be used to fetch a token or verification key from UAA in a local BOSH Lite deployment.

### Prerequisites for testing these example clients with BOSH Lite

- Add IP of UAA your /etc/hosts (can be found using `bosh vms`)

		10.244.0.134 uaa.service.cf.internal

- In your deployment manifest for cf-release configure UAA to listen on TLS by specifying the port, certificate, and key with the following properties:

		properties:
		  uaa:
		    ssl:
		      port: 8443
		    sslCertificate: |
		      -----BEGIN CERTIFICATE-----
		      { ... }
		      -----END CERTIFICATE-----
		    sslPrivateKey: |
		      -----BEGIN RSA PRIVATE KEY-----
		      { ... }
		      -----END RSA PRIVATE KEY-----


- Assuming the cert you've configured for UAA is self-signed, provide `true` for the `skip-verification` option

### Fetch token
This client connects to UAA using https and fetches a token.

```
Usage: <client-name> <client-secret> <uaa-url> <skip-verification>
```

Example
```
$ go run examples/fetch_token.go gorouter gorouter-secret https://uaa.service.cf.internal:8443 true

Connecting to: https://uaa.service.cf.internal:8443 ...
Response:
	token: eyJhbGciOiJSUzI1NiJ9.eyJqdGkiOiJlOGQ3NWJiNi1kMGMxLTRmMjEtYWMyMy05ZGRiNmY2MWI3ZjkiLCJzdWIiOiJnb3JvdXRlciIsImF1dGhvcml0aWVzIjpbInJvdXRpbmcucm91dGVzLnJlYWQiXSwic2NvcGUiOlsicm91dGluZy5yb3V0ZXMucmVhZCJdLCJjbGllbnRfaWQiOiJnb3JvdXRlciIsImNpZCI6Imdvcm91dGVyIiwiYXpwIjoiZ29yb3V0ZXIiLCJncmFudF90eXBlIjoiY2xpZW50X2NyZWRlbnRpYWxzIiwicmV2X3NpZyI6IjdmNTE1MmQyIiwiaWF0IjoxNDU0NzA5NTUxLCJleHAiOjE0NTQ3NTI3NTEsImlzcyI6Imh0dHBzOi8vdWFhLmJvc2gtbGl0ZS5jb20vb2F1dGgvdG9rZW4iLCJ6aWQiOiJ1YWEiLCJhdWQiOlsiZ29yb3V0ZXIiLCJyb3V0aW5nLnJvdXRlcyJdfQ.QSdLbdhDFWQXSJ3lPbTVUCj6zEH1DUPU3V-x8lX48qOPg99snalEEIBX5y5Ki6mZLWJ9p6UUIH1xANz4mGATcBIO282wcRBK0Pbc-r1OkjFNJTvwdV75kP9ovbGXGNbQZMksEvEtgOQ_icz7XsJrkTxtV29uPYDpKHbxtvqpPeU
	expires: 43199
```

### Fetch key
This client connects to UAA using https and fetches the UAA verification key. An Oauth client is not required as the target API endpoint on UAA does not require authentication.

```
Usage: <uaa-url> <skip-verification>
```

Example
```
$ go run examples/fetch_key.go https://uaa.service.cf.internal:8443 true

Connecting to: https://uaa.service.cf.internal:8443 ...
Response:
	token: eyJhbGciOiJSUzI1NiJ9.eyJqdGkiOiJlOGQ3NWJiNi1kMGMxLTRmMjEtYWMyMy05ZGRiNmY2MWI3ZjkiLCJzdWIiOiJnb3JvdXRlciIsImF1dGhvcml0aWVzIjpbInJvdXRpbmcucm91dGVzLnJlYWQiXSwic2NvcGUiOlsicm91dGluZy5yb3V0ZXMucmVhZCJdLCJjbGllbnRfaWQiOiJnb3JvdXRlciIsImNpZCI6Imdvcm91dGVyIiwiYXpwIjoiZ29yb3V0ZXIiLCJncmFudF90eXBlIjoiY2xpZW50X2NyZWRlbnRpYWxzIiwicmV2X3NpZyI6IjdmNTE1MmQyIiwiaWF0IjoxNDU0NzA5NTUxLCJleHAiOjE0NTQ3NTI3NTEsImlzcyI6Imh0dHBzOi8vdWFhLmJvc2gtbGl0ZS5jb20vb2F1dGgvdG9rZW4iLCJ6aWQiOiJ1YWEiLCJhdWQiOlsiZ29yb3V0ZXIiLCJyb3V0aW5nLnJvdXRlcyJdfQ.QSdLbdhDFWQXSJ3lPbTVUCj6zEH1DUPU3V-x8lX48qOPg99snalEEIBX5y5Ki6mZLWJ9p6UUIH1xANz4mGATcBIO282wcRBK0Pbc-r1OkjFNJTvwdV75kP9ovbGXGNbQZMksEvEtgOQ_icz7XsJrkTxtV29uPYDpKHbxtvqpPeU
	expires: 43199
```
