# Known Issue: Multiple Expect 100-continue responses

## 🐛 Bug 1 Summary
Previously clients that sent a request with the header “Expect: 100-continue” 
only got one response back with status code 100 before getting their final 
response with the “real” status code. With go 1.20 (before the fix) the client got 
two responses with status code 100. According to the HTTP 1.1 RFC, clients 
should be able to handle multiple responses with status code 100, however, 
some java spring clients have been reported to throw exceptions when this happens.

## 🐞 Bug 2 Summary
Previously when clients sent a request with the header “Expect: 100-continue”, 
the gorouter's access log entry had the status code of the final response, 
not “100”. With go 1.20 (before the fix) the access log shows a status code of “100” 
and would never log the final status code. This made it impossible for operators to 
look at response codes to be able to successfully monitor the health of their deployments.

## 🔥 Affected Versions
* Routing-release versions 0.258.0 - 0.272.0
* Routing-release version 0.273.0 has the fix for bug 1.
* Routing-release version 0.274.0 will have the fix for bug 2.

## 🔥 Affected Users
### Bug 1 affects users who...
* use java spring as a client to send data to apps on Cloud Foundry through Gorouter with the "Expect: 100-continue" header.
* use other clients that use other frameworks that are not RFC compliant

### Bug 2 affects users who…
* have apps on CF where data is sent to those apps with the "Expect: 100-continue" header.
* Basically everyone.

## 🔨 Mitigations
* upgrade your CF deployment to use routing-release  0.274.0 or later.

# ⚠️ Stop here unless you are interested in the nitty gritty details

## Bug 1 Root Cause Analysis 
Gorouter is a go reverse proxy that has a custom [transport](https://pkg.go.dev/net/http#Transport). 
The transport has a ExpectContinueTimeout property. 
Gorouter did not set this property explicitly, so it defaulted to 0 seconds. 
This means that when gorouter received a request with an expect 100-continue header, 
it would immediately respond to the client with a response with status code 100, 
without waiting for the server app to respond. 
The server app would eventually send its own response with status code 100. 
How the underlying reverse proxy handled this response from the server app changed from Go 1.19 to Go 1.20.

In Go 1.19, the reverse proxy does not proxy the response with status code 100 
from the server app. This means that the client only got one response with status code 100.

In Go 1.20, the reverse proxy does proxy the response with status code 100 from the server app. 
This means that the client gets two responses with status code 100: one immediately from gorouter 
and one from the server app.

[This change was purposefully done](https://github.com/golang/go/issues/26088) by the go contributors to be more compliant with the HTTP 1.1 RFC.

There is an easy fix for this: set the ExpectContinueTimeout to 1 second. 
When this value is set Gorouter will no longer send a response with status code 100 right away.
Instead it will wait to see if the app sends the response first.

However, if the server app takes more than 1 second to send a response with status code 100,
then there is a chance that the client will again get 2 responses with status code 100.

## 📖 RFC Says
[The RFC says that proxies like gorouter must not filter 1XX responses.]([url](https://datatracker.ietf.org/doc/html/rfc7231#section-6.2))

> “A proxy MUST forward 1xx responses unless the proxy itself requested 
> the generation of the 1xx response.  For example, if a proxy adds an 
> "Expect: 100-continue" field when it forwards a request, then it need 
> not forward the corresponding 100 (Continue) response(s).”

👉 This means that in go 1.19 the reverse proxy (and thus gorouter) 
was not compliant with this spec. Now in go 1.20 it is compliant and will forward on 1XX headers from apps.

[There is also a section about multiple 1xx responses.](https://www.rfc-editor.org/rfc/rfc7231#section-6.2)

> “A client MUST be able to parse one or more 1xx responses received prior
> to a final response, even if the client does not expect one. A user agent
> MAY ignore unexpected 1xx responses.”

👉 This says that clients should be able to handle multiple 1xx responses.

## Steps to reproduce in Cloud Foundry
1. Push the proxy example app
1. Generate a large-ish file to upload
   ```
   curl PROXY-APP-URL/download/870912 > large-file
   ```
1. Upload the file with the Expect header.
   ```
   curl PROXY-APP-URL/upload --data-binary @large-file -H "Expect: 100-continue" -v
   ```
1. If you are on a broken env you will see two status code 100 responses (edited response below for brevity and clarity)
   ```
   $ curl potato.apps.pewterblue.cf-app.com/upload -d @foo -H "Expect: 100-continue" -v
   
   < HTTP/1.1 100 Continue
   < HTTP/1.1 100 Continue
   < HTTP/1.1 200 OK
   
   430387 bytes received and read
   ```
1. If you are are on a broken env, you will see a log with status code 100 in the app's logs
   ```
   $ cf logs potato
   Retrieving logs for app potato in org o / space s as admin...
   
   2023-06-29T21:28:49.83+0000 [RTR/0] OUT potato.minionyellow.cf-app.com - [2023-06-29T21:28:49.793188099Z] "POST /upload HTTP/1.1" 100 
   ```
1. If you are on a working env, you will see one status code 100 responses (edited response below for brevity and clarity)
   ```
   $ curl potato.apps.lightgoldenrodyellow.cf-app.com/upload -d @foo -H "Expect: 100-continue" -v
   
   < HTTP/1.1 100 Continue
   < HTTP/1.1 200 OK
   
   430387 bytes received and read
   ```
1. If you are are on a working env, you will see a log with status code 200 in the app's logs
   ```
   $ cf logs potato
   Retrieving logs for app potato in org o / space s as admin...
   
   2023-06-29T21:28:49.83+0000 [RTR/0] OUT potato.minionyellow.cf-app.com - [2023-06-29T21:28:49.793188099Z] "POST /upload HTTP/1.1" 200 
   ```
## Steps to reproduce in Go only
[Follow the steps in this gist.](https://gist.github.com/ameowlia/f061d4d1f07e89139f6874aea5590246)

## Mapping out the interactions between Go versions and reverse proxy configuration settings

![image](https://github.com/cloudfoundry/routing-release/assets/7025605/e0e02652-202e-4c64-9715-1081e63aa53b)
