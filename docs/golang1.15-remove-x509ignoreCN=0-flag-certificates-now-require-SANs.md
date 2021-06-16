# golang 1.15 X.509 CommonName deprecation

This doc helps operators understand why certificates used by network Load
Balancers and the gorouter to serve TLS traffic must contain at least one
Subject Alternative Name (SAN).

## 🔥 Version
This deprecation is firmly observed in [0.215.0](https://github.com/cloudfoundry/routing-release/releases/tag/0.215.0).

## 📑 Context

When golang 1.15 was released, the authors added a deprecation along with an
environment variable to temporarily bypass the feature deprecation around the
use of `CommonName` in x.509 Certificates:

  > The deprecated, legacy behavior of treating the CommonName field on X.509 certificates as a host name when no Subject Alternative Names are present is now disabled by default. It can be temporarily re-enabled by adding the value x509ignoreCN=0 to the GODEBUG environment variable.
  >
  > Note that if the CommonName is an invalid host name, it's always ignored, regardless of GODEBUG settings. Invalid names include those with any characters other than letters, digits, hyphens and underscores, and those with empty labels or trailing dots.

Source: [Go 1.15 Release Notes](https://golang.org/doc/go1.15#commonname).


In [routing-release
0.209.0](https://github.com/cloudfoundry/routing-release/releases/tag/0.209.0),
the team bumped the version of golang used to `1.15.6` with the
`x509ignoreCN=0` flag to get the golang upgrade started.

Now, with [routing-release
0.215.0](https://github.com/cloudfoundry/routing-release/releases/tag/0.215.0),
we have removed the use of the `x509ignoreCN=0` flag to stay ahead of the golang
release curve, now requiring that operators have compatible certificates.

## 🤔 What does this mean for operators?
### 1️⃣ Gorouter TLS Certificates

If an operator has configured `routing-release` by enabling the
`router.enable_ssl` [bosh
property](https://github.com/cloudfoundry/routing-release/blob/1de3053a8b3b6d3169ac53729832fb51c93fc1ac/jobs/gorouter/spec#L90-L92)
to serve TLS for the foundation:

```yaml
  router.enable_ssl:
    description: "When enabled, Gorouter will listen on port 443 and terminate TLS for requests received on this port."
    default: false
```

Then the certificate(s) provided in the `router.tls_pem` [bosh
property](https://github.com/cloudfoundry/routing-release/blob/1de3053a8b3b6d3169ac53729832fb51c93fc1ac/jobs/gorouter/spec#L116-L126)
(shown below) must contain `Subject Alternative Name(s)` (`SANs`), including the
same domain that would be set in the `CommonName`.

```yaml
  router.tls_pem:
    description: "Array of private keys and certificates for serving TLS requests. Each element in the array is an object containing fields 'private_key' and 'cert_chain', each of which supports a PEM block. Required if router.enable_ssl is true."
    example: |
      - cert_chain: |
          -----BEGIN CERTIFICATE-----
          -----END CERTIFICATE-----
          -----BEGIN CERTIFICATE-----
          -----END CERTIFICATE-----
        private_key: |
          -----BEGIN RSA PRIVATE KEY-----
          -----END RSA PRIVATE KEY-----
```

### 2️⃣  Network Load Balancers

If the foundation leverages a network Load Balancer that includes a certificate
to forward TLS traffic to the router, the operator *must* ensure the certificate
for the Load Balancer includes a SAN. Follow the procedure in [How to check a
certificate for SANs](#--how-to-check-a-certificate-for-subject-alternative-names-sans) to
validate that the certificate used contains the appropriate SAN(s).

## 📝 👩‍🔬 How to check a certificate for Subject Alternative Names (SANs)
Operators can check if their certificates contain a SAN by running the following
command and looking in the output for values in the `X509v3 Subject Alternative Name:` field:

```bash
  $ openssl x509 -noout -text -in gorouter_tls_cert.pem
```

```bash
  Certificate:
      Data:
	  Version: 3 (0x2)
	  Serial Number:
	      78:59:af:76:7f:32:7b:34:d6:99:e4:d0:4b:cc:4c:c7:a0:95:ea:83
	  Signature Algorithm: sha256WithRSAEncryption
	  Issuer: CN = *.no-sans-env.funtime.lol
	  Validity
	      Not Before: Jun  2 19:29:31 2021 GMT
	      Not After : May 31 19:29:31 2031 GMT
	  Subject: CN = *.no-sans-env.funtime.lol # MUST BE INCLUDED IN Subject Alternative Names (SANs)
	  Subject Public Key Info:
	      Public Key Algorithm: rsaEncryption
		  RSA Public-Key: (2048 bit)
		  Modulus:
		      00:e5:78:42:a3:38:ff:bd:fb:1d:b2:2d:f0:ba:17:
		      ....
		      ....
		      ....
		      d7:af:65:e9:c5:c4:53:ec:a7:01:84:df:09:0b:e6:
		  Exponent: 65537 (0x10001)
	  X509v3 extensions:
	      X509v3 Subject Key Identifier:
		  06:3A:D9:D4:74:11:2A:92:17:48:BC:D5:71:C2:A3:88:4B:F6:D0:C2
	      X509v3 Authority Key Identifier:
		  keyid:06:3A:D9:D4:74:11:2A:92:17:48:BC:D5:71:C2:A3:88:4B:F6:D0:C2

	      X509v3 Basic Constraints: critical
		  CA:TRUE
	      X509v3 Subject Alternative Name:
		  DNS:*.no-sans-env.funtime.lol
#### Need to have this final property ↑ that matches the CommonName (CN)
```
