package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func frontend(t *testing.T, upstream string) (*http.Client, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Synthetic load test"}, DNSNames: []string{"public.load.test", "admin.load.test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IsCA: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	server := httptest.NewUnstartedServer(nil)
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	host := "public.load.test:" + port
	handler, closeIdle, err := newProxy(proxyConfig{host: host, upstream: upstream})
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = handler
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	server.StartTLS()
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	t.Cleanup(func() { transport.CloseIdleConnections(); closeIdle(); server.Close() })
	return &http.Client{Transport: transport, Timeout: 3 * time.Second}, "https://" + host
}

func TestTLSProxyStreamsSSEBeforeUpstreamCloses(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: cue\ndata: {\"revision\":7}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer upstream.Close()
	client, origin := frontend(t, upstream.URL)
	response, err := client.Get(origin + "/api/v1/display/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	var frame strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		frame.WriteString(line)
		if line == "\n" {
			break
		}
	}
	if frame.String() != "event: cue\ndata: {\"revision\":7}\n\n" {
		t.Fatal("SSE bytes changed", frame.String())
	}
	if response.TLS == nil || len(response.TLS.VerifiedChains) == 0 {
		t.Fatal("frontend TLS was not verified")
	}
	// release remains open: receiving this frame cannot depend on stream completion.
}

func TestTLSProxyPreservesNativeIntentAndReplacesForgedForwarding(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if r.Method != "PUT" || r.URL.RequestURI() != "/api/v1/change?version=2" || string(body) != "{\"action\":\"save\"}" {
			t.Error("native request changed")
		}
		for key, want := range map[string]string{"X-CSRF-Token": "synthetic-csrf", "Idempotency-Key": "synthetic-original-intent", "Cookie": "native_session=synthetic", "If-Match": "\"2\"", "X-Forwarded-Proto": "https"} {
			if r.Header.Get(key) != want {
				t.Errorf("%s changed", key)
			}
		}
		if r.Header.Get("Forwarded") != "" || r.Header.Get("X-Forwarded-Host") != r.Host || strings.Contains(r.Header.Get("X-Forwarded-For"), "attacker") {
			t.Error("forged forwarding survived")
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Set-Cookie", "native_session=synthetic; Secure; HttpOnly; SameSite=Lax")
		_, _ = io.WriteString(w, "{\"acknowledged_version\":3}")
	}))
	defer upstream.Close()
	client, origin := frontend(t, upstream.URL)
	request, _ := http.NewRequest("PUT", origin+"/api/v1/change?version=2", strings.NewReader("{\"action\":\"save\"}"))
	for key, value := range map[string]string{"X-CSRF-Token": "synthetic-csrf", "Idempotency-Key": "synthetic-original-intent", "Cookie": "native_session=synthetic", "If-Match": "\"2\"", "Forwarded": "for=attacker;proto=http", "X-Forwarded-For": "attacker", "X-Forwarded-Host": "attacker", "X-Forwarded-Proto": "http"} {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 || string(body) != "{\"acknowledged_version\":3}" || calls.Load() != 1 || response.Header.Get("Cache-Control") != "no-store" || !strings.Contains(response.Header.Get("Set-Cookie"), "Secure; HttpOnly") {
		t.Fatal("native acknowledgement/cookie altered")
	}
}

func TestTLSProxyRejectsAlternateHostBeforeUpstream(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(200) }))
	defer upstream.Close()
	client, origin := frontend(t, upstream.URL)
	for _, changeSNI := range []bool{false, true} {
		request, _ := http.NewRequest("GET", origin+"/readyz", nil)
		if changeSNI {
			request.URL.Host = strings.Replace(request.URL.Host, "public.", "admin.", 1)
			request.Host = strings.TrimPrefix(origin, "https://")
		} else {
			request.Host = "admin.load.test:" + strings.Split(origin, ":")[2]
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusMisdirectedRequest || calls.Load() != 0 {
			t.Fatal("alternate HTTP Host or SNI reached upstream")
		}
	}
}

func TestTLSProxyNeverReplaysLostBodylessWrite(t *testing.T) {
	var effects atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			_, _ = io.WriteString(w, "warm connection")
			return
		}
		if r.Header.Get("Idempotency-Key") != "synthetic-original-intent" {
			t.Error("native intent key removed")
		}
		if effects.Add(1) == 1 {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = connection.Close()
			return
		}
		_, _ = io.WriteString(w, "a replay would look successful")
	}))
	defer upstream.Close()
	client, origin := frontend(t, upstream.URL)
	warm, err := client.Get(origin + "/warm")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, warm.Body)
	_ = warm.Body.Close()
	request, _ := http.NewRequest("POST", origin+"/native-effect", nil)
	request.Header.Set("Idempotency-Key", "synthetic-original-intent")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusBadGateway || effects.Load() != 1 || string(body) != "upstream unavailable\n" {
		t.Fatalf("lost original response was replayed or relabelled: status=%d effects=%d", response.StatusCode, effects.Load())
	}
}

func TestTLSProxyConfigurationRejectsAmbientOrCredentialTargets(t *testing.T) {
	for _, config := range []proxyConfig{{host: "public.load.test:8443", upstream: "https://api:8080"}, {host: "public.load.test:8443", upstream: "http://user:secret@api:8080"}, {host: "public.load.test:8443", upstream: "http://api:8080/path"}, {host: "public.load.test:8443", upstream: "http://api:8080?token=secret"}, {host: "public.load.test", upstream: "http://api:8080"}, {host: "127.0.0.1:8443", upstream: "http://api:8080"}} {
		if _, _, err := newProxy(config); err == nil {
			t.Fatal("invalid named/private proxy accepted")
		}
	}
}
