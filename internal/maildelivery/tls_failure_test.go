package maildelivery

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"testing"
	"time"
)

func TestTLSFailureBeforeDATAKeepsClosedTransportRetryable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	hello := make(chan error, 1)
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		peer, err := listener.Accept()
		if err != nil {
			hello <- err
			return
		}
		defer func() { _ = peer.Close() }()
		_ = peer.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, err = peer.Read(make([]byte, 4096))
		hello <- err
		<-stop
	}()
	raw, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	secure := tls.Client(raw, &tls.Config{ServerName: "localhost", MinVersion: tls.VersionTLS12})
	handshake := make(chan error, 1)
	go func() { handshake <- secure.Handshake() }()
	if err = <-hello; err != nil {
		t.Fatal(err)
	}
	// Send's context.AfterFunc closes this same underlying connection. Closing
	// before its socket deadline deterministically exercises the competing path.
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}
	err = <-handshake
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("expected actual closed TCP transport, got %v", err)
	}
	receipt := tlsFailure(err, "synthetic-closed-transport")
	if receipt.State != Retryable || receipt.Code != "relay_unavailable" {
		t.Fatalf("pre-DATA closed transport classified %s/%s", receipt.State, receipt.Code)
	}
	invalidCertificate := &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}
	receipt = tlsFailure(invalidCertificate, "synthetic-untrusted-certificate")
	if receipt.State != Permanent || receipt.Code != "tls_verification_failed" {
		t.Fatal("certificate failure lost its permanent rejection")
	}
}

func TestSMTPCanceledSTARTTLSBeforeDATARemainsRetryable(t *testing.T) {
	relay := startRelay(t, "starttls_timeout", "starttls")
	settings := relay.settings
	// Cancel at the protocol boundary, before the configured socket deadline.
	// Production timeout bounds and the existing deadline test are unchanged.
	settings.Timeout = 3 * time.Second
	provider, err := NewSMTP(settings)
	if err != nil {
		t.Fatal(err)
	}
	provider.roots = relay.roots
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	outcome := make(chan Receipt, 1)
	go func() { outcome <- provider.Send(ctx, personal()) }()
	select {
	case <-relay.tlsStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("fixture never reached STARTTLS")
	}
	cancel()
	var receipt Receipt
	select {
	case receipt = <-outcome:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled SMTP send did not stop")
	}
	if receipt.State != Retryable || receipt.Code != "relay_unavailable" {
		t.Fatalf("canceled pre-DATA TLS classified %s/%s", receipt.State, receipt.Code)
	}
	relay.stop()
	evidence := <-relay.done
	if evidence.authenticated || len(evidence.recipients) != 0 || len(evidence.body) != 0 {
		t.Fatal("canceled TLS exchanged authentication, recipients or DATA")
	}
	unknown := tlsFailure(
		errors.New("synthetic unknown TLS protocol failure"),
		"synthetic-unknown-tls",
	)
	if unknown.State != Permanent || unknown.Code != "tls_verification_failed" {
		t.Fatal("unknown TLS error was broadened to a retryable network failure")
	}
}
