# External TLS proxy for the private capacity fixture

This component terminates verified TLS and forwards to a single declared private
HTTP upstream. It does not create users, sessions, facts or worker clocks. The
normal public/admin/worker images remain unchanged. Passing component tests is
not a native installation or capacity report.

Build with this directory as the context so private fixture files and the rest
of the workspace are excluded:

```sh
docker build -t hackbox-load-tls:reviewed docs/verification/load/tlsproxy
```

The integrated runner must record the source and immutable image ID. Use three
instances on the private fixture network, each with the matching DNS alias and
explicit listener. Publish each port only on loopback for the verified host driver.

| Instance | `-listen` | `-host` | `-upstream` |
| --- | --- | --- | --- |
| Public | `:8443` | `public.load.test:8443` | `http://api:8080` |
| Administration | `:8444` | `admin.load.test:8444` | `http://admin:8080` |
| Synthetic issuer | `:8445` | `issuer.load.test:8445` | `http://issuer:8080` |

Pass `-cert /tls/server.pem -key /tls/server-key.pem` on each instance. Generate
an ephemeral fixture CA and a server certificate with all three DNS names;
clients must verify both that CA and the original hostname/SNI while their
explicit dial map selects the loopback listener. Mount the certificate and
private key read-only; UID10001 must have the required access. Keep the private
key outside source/build contexts, images and public reports. Normal application
processes must trust the fixture CA for issuer requests. Preserve the declared
external HTTPS origins in app/OIDC configuration, and resolve their network aliases
to the corresponding proxy instances. Clear ambient HTTP proxy settings in this
isolated deployment; the proxy component itself never uses an ambient proxy.

The proxy rejects a different HTTP Host or TLS SNI before reaching an upstream.
It removes untrusted forwarding headers, then derives them from the actual
request. Native cookies, CSRF/version/intent headers, bodies, status codes,
receipts and cookie security attributes pass through. Upstream failures return a
bounded502 response without logging authenticated URLs, codes or bodies.

SSE frames flush while the upstream connection remains open. No response write
timeout limits the sustained stream; request-header/body bounds and shutdown
remain finite. The integrated profile must still demonstrate actual authorised
SSE receipt and measure its unchanged2s target.

Safe reads use pooled upstream HTTP connections. Mutations use fresh private
HTTP/1 connections, retaining the native Idempotency-Key while disabling upstream
keep-alive reuse. This prevents Go's transport from automatically replaying an
already-processed bodyless keyed write after a lost response. The profile includes
this connection cost in native request latency and resource usage; it is not a
provider exclusion. A502 after a possible effect remains uncertain in the native
driver and must be reconciled against its original intent.

```sh
GOMAXPROCS=2 go test -race ./docs/verification/load/tlsproxy -count=1
```

Tests use real loopback TLS and HTTP servers: a frame arrives before its stream
closes, native intent/security headers and original acknowledgements survive,
alternate hosts never reach the backend, and a lost keyed write has one effect.
The original shared-transport implementation was observed returning200 after two
effects in the last case; the separate mutation transport returns502 after one.
Those are component controls, not the five-event/1,000-active-person/1,800s proof.
