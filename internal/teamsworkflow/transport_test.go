package teamsworkflow

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

const (
	testHost = "syntheticenv.01.environment.api.powerplatform.com"
	endpoint = "https://" + testHost + "/powerautomate/automations/direct/workflows/00000000000040008000000000000001/triggers/manual/paths/invoke?api-version=1&sp=%2Ftriggers%2Fmanual%2Frun&sv=1.0&sig=SYNTHETIC_SIGNATURE_NEVER_LOGGED"
)

type lookupFunc func(context.Context, string, string) ([]netip.Addr, error)

func (f lookupFunc) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	return f(ctx, network, host)
}

func TestEndpointPolicy(t *testing.T) {
	for _, raw := range []string{endpoint, strings.Replace(endpoint, testHost, "default00000000000040008000000000000001.environment.api.powerplatform.com", 1), strings.Replace(endpoint, testHost, testHost+":443", 1)} {
		if _, ok := validateEndpoint(raw); !ok {
			t.Fatal("current endpoint rejected")
		}
	}
	for name, raw := range map[string]string{
		"http": strings.Replace(endpoint, "https:", "http:", 1), "userinfo": strings.Replace(endpoint, "https://", "https://name:password@", 1), "port": strings.Replace(endpoint, testHost, testHost+":8443", 1), "legacy": strings.Replace(endpoint, testHost, "prod-01.uksouth.logic.azure.com", 1), "connector": strings.Replace(endpoint, testHost, "tenant.webhook.office.com", 1), "suffix": strings.Replace(endpoint, testHost, testHost+".attacker.example", 1), "otherMicrosoft": strings.Replace(endpoint, testHost, "login.microsoftonline.com", 1), "fragment": endpoint + "#", "querytoken": endpoint + "&token=private", "duplicates": endpoint + "&sig=other", "pathescape": strings.Replace(endpoint, "/powerautomate/", "/%70owerautomate/", 1), "otherAPI": strings.Replace(endpoint, "/automations/direct/workflows/", "/admin/", 1), "version": strings.Replace(endpoint, "api-version=1", "api-version=2099", 1), "unsigned": strings.Split(endpoint, "&sig=")[0],
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := validateEndpoint(raw); ok {
				t.Fatal("unsafe or unsupported endpoint allowed")
			}
		})
	}
}

func TestPublicAddresses(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "0.0.0.0", "10.0.0.1", "172.16.0.1", "192.168.0.1", "169.254.169.254", "100.64.0.1", "192.0.2.1", "198.51.100.2", "203.0.113.3", "198.18.0.1", "224.0.0.1", "255.255.255.255", "::", "::1", "::ffff:8.8.8.8", "fe80::1%en0", "fc00::1", "5f00::1", "fec0::1", "100:0:0:1::1", "2001:db8::1", "2002:0808:0808::1", "64:ff9b::0808:0808", "ff02::1"} {
		t.Run(value, func(t *testing.T) {
			if publicAddress(netip.MustParseAddr(value)) {
				t.Fatal("unsafe address allowed")
			}
		})
	}
	for _, value := range []string{"8.8.8.8", "20.50.1.2", "2603:1030:405:3::1"} {
		if !publicAddress(netip.MustParseAddr(value)) {
			t.Fatal("public address rejected")
		}
	}
}

func tlsFixture(t *testing.T, handler http.Handler) (*adapter, *atomic.Int32) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: testHost},
		DNSNames:     []string{testHost},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	rawKey, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: rawKey}),
	)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}}
	server.StartTLS()
	t.Cleanup(server.Close)
	roots := x509.NewCertPool()
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots.AddCert(parsed)
	calls := &atomic.Int32{}
	a := &adapter{
		roots: roots,
		lookup: lookupFunc(func(_ context.Context, network, host string) ([]netip.Addr, error) {
			calls.Add(1)
			if network != "ip" || host != testHost {
				t.Error("unexpected DNS request")
			}
			return []netip.Addr{netip.MustParseAddr("20.50.1.2")}, nil
		}),
		dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != "20.50.1.2:443" || network != "tcp" {
				t.Error("DNS address was not pinned")
			}
			return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
		},
	}
	return a, calls
}

func TestPinnedTLSNoProxyNoRedirectAndProviderClassification(t *testing.T) {
	var requests atomic.Int32
	var status atomic.Int32
	status.Store(202)
	a, dns := tlsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" ||
			r.Method != "POST" {
			t.Error("unexpected provider authentication or method")
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Retry-After", "12")
		w.Header().Set("Location", "https://127.0.0.1/private")
		w.WriteHeader(int(status.Load()))
	}))
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	for _, sample := range []struct {
		status  int
		outcome jobs.Outcome
		reason  string
	}{{202, jobs.Delivered, "provider_accepted"}, {429, jobs.Retryable, "rate_limited"}, {401, jobs.Permanent, "provider_rejected"}, {403, jobs.Permanent, "provider_rejected"}, {400, jobs.Permanent, "provider_rejected"}, {302, jobs.Permanent, "redirect_rejected"}, {500, jobs.Ambiguous, "uncertain_acceptance"}, {408, jobs.Ambiguous, "uncertain_acceptance"}} {
		status.Store(int32(sample.status))
		before := requests.Load()
		got := a.send(t.Context(), endpoint, []byte(`{"type":"message"}`))
		if got.Outcome != sample.outcome || got.Reason != sample.reason ||
			got.Status != sample.status {
			t.Fatalf("classification: %#v", got)
		}
		if requests.Load() != before+1 {
			t.Fatal("request replayed or redirected")
		}
		if sample.status == 429 && got.RetryAfter != 12*time.Second {
			t.Fatal("Retry-After ignored")
		}
	}
	if dns.Load() != 8 {
		t.Fatal("DNS must resolve exactly once per attempt")
	}
}

func TestRejectsUnsafeDNSAndDoesNotDial(t *testing.T) {
	var dialed bool
	a := &adapter{lookup: lookupFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{
			netip.MustParseAddr("20.50.1.2"),
			netip.MustParseAddr("169.254.169.254"),
		}, nil
	}), dial: func(context.Context, string, string) (net.Conn, error) {
		dialed = true
		return nil, errors.New("must not dial")
	}}
	got := a.send(t.Context(), endpoint, []byte(`{}`))
	if got.Outcome != jobs.Permanent || got.Reason != "unsafe_destination" || dialed {
		t.Fatal("mixed public/private DNS was allowed")
	}
}

func TestLostResponseAfterRequestIsAmbiguous(t *testing.T) {
	a, _ := tlsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	}))
	got := a.send(t.Context(), endpoint, []byte(`{"synthetic":"body"}`))
	if got.Outcome != jobs.Ambiguous || got.Reason != "uncertain_response" {
		t.Fatalf("unsafe replay classification: %#v", got)
	}
}

func TestRejectsLongRateLimitRatherThanRetryingBeforeProviderWindow(t *testing.T) {
	a, _ := tlsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "86400")
		w.WriteHeader(429)
	}))
	got := a.send(t.Context(), endpoint, []byte(`{}`))
	if got.Outcome != jobs.Permanent || got.Reason != "rate_limit_requires_review" {
		t.Fatal("long rate window truncated into an early retry")
	}
}

func TestTLSHostnameAndTrustAreNeverBypassed(t *testing.T) {
	var requests atomic.Int32
	a, _ := tlsFixture(
		t,
		http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) { requests.Add(1); w.WriteHeader(202) },
		),
	)
	a.lookup = lookupFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("20.50.1.2")}, nil
	})
	wrongHost := strings.Replace(
		endpoint,
		testHost,
		"anotherenv.01.environment.api.powerplatform.com",
		1,
	)
	got := a.send(t.Context(), wrongHost, []byte(`{}`))
	if got.Outcome != jobs.Retryable || got.Reason != "connection_unavailable" ||
		requests.Load() != 0 {
		t.Fatal("TLS hostname verification bypassed")
	}
	a.roots = nil
	got = a.send(t.Context(), endpoint, []byte(`{}`))
	if got.Outcome != jobs.Retryable || got.Reason != "connection_unavailable" ||
		requests.Load() != 0 {
		t.Fatal("TLS trust verification bypassed")
	}
}
