package loadprofile

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// This is a real TLS transport boundary, not native authentication or load
// evidence. The server certificate's external name differs from its loopback
// listener, so an insecure or hostname-rewriting implementation is detectable.
func TestVerifiedTransportPreservesIdentityAndRejectsEscapes(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, r.Host)
	}))
	defer server.Close()
	certificate := server.Certificate()
	if len(certificate.DNSNames) == 0 {
		t.Fatal("TLS fixture requires external certificate name")
	}
	name := certificate.DNSNames[0] + ":8443"
	destination := server.Listener.Addr().String()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	targets := map[string]string{name: destination, "wrong-name.invalid:8443": destination}
	transport, err := NewVerifiedTransport(ca, targets)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	// Mutating caller configuration cannot retarget an authenticated session.
	targets[name] = "127.0.0.1:1"
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	response, err := client.Get("https://" + name + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || string(body) != name {
		t.Fatal("external Host identity was replaced", err, string(body))
	}
	for _, target := range []string{"http://" + name, "https://undeclared.invalid:8443", "https://wrong-name.invalid:8443", "https://user:password@" + name} {
		t.Run(target, func(t *testing.T) {
			if response, err := client.Get(target); err == nil {
				_ = response.Body.Close()
				t.Fatal("unverified target accepted")
			}
		})
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+name, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "alternate.invalid:8443"
	if _, err := client.Do(request); err == nil {
		t.Fatal("alternate Host accepted")
	}
	wrong, err := NewVerifiedTransport(
		unrelatedTransportCA(t),
		map[string]string{name: destination},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.CloseIdleConnections()
	if response, err := (&http.Client{Transport: wrong, Timeout: 3 * time.Second}).Get(
		"https://" + name,
	); err == nil {
		_ = response.Body.Close()
		t.Fatal("untrusted server CA accepted")
	}
	if requests.Load() != 1 {
		t.Fatal("refused target reached HTTP handler", requests.Load())
	}
}

func unrelatedTransportCA(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
	}
	raw, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw})
}

func TestVerifiedTransportRequiresDeclaredLoopbackTargets(t *testing.T) {
	ca := unrelatedTransportCA(t)
	for _, destination := range []string{"127.0.0.1", "localhost:8443", "192.0.2.1:8443", "127.0.0.1:0", "127.0.0.1:08443"} {
		if _, err := NewVerifiedTransport(
			ca,
			map[string]string{"fixture.invalid:8443": destination},
		); err == nil {
			t.Fatal("ambiguous or non-loopback destination accepted", destination)
		}
	}
	if _, err := NewVerifiedTransport(
		[]byte(strings.Repeat("invalid", 10)),
		map[string]string{"fixture.invalid:8443": "127.0.0.1:8443"},
	); err == nil {
		t.Fatal("missing CA accepted")
	}
}

func TestVerifiedTransportDoesNotReplayBodylessUnknownCommand(t *testing.T) {
	var effects atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, "warm idle connection")
			return
		}
		if r.Header.Get("Idempotency-Key") != "synthetic-original-key" {
			t.Error("native command identity stripped")
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
		_, _ = io.WriteString(w, "second effect")
	}))
	defer server.Close()
	name := server.Certificate().DNSNames[0] + ":8443"
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	transport, err := NewVerifiedTransport(
		ca,
		map[string]string{name: server.Listener.Addr().String()},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	response, err := client.Get("https://" + name)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://"+name, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Idempotency-Key", "synthetic-original-key")
	response, err = client.Do(request)
	if response != nil {
		_ = response.Body.Close()
	}
	if err == nil || effects.Load() != 1 {
		t.Fatalf(
			"unknown bodyless command was replayed: effects=%d transport_error=%v",
			effects.Load(),
			err,
		)
	}
}
