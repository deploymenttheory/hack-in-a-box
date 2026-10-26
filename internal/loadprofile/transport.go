package loadprofile

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

// VerifiedTransport keeps fixture DNS mapping separate from TLS identity. URLs,
// SNI and certificate checks retain the declared external hostname; only the
// TCP destination is pinned to an explicit loopback port. There is no ambient
// HTTP_PROXY, DNS fallback, plaintext escape or system-root fallback.
type VerifiedTransport struct {
	transport *http.Transport
	commands  *http.Transport
	targets   map[string]string
}

func NewVerifiedTransport(caPEM []byte, targets map[string]string) (*VerifiedTransport, error) {
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) || len(targets) == 0 || len(targets) > 3 {
		return nil, fmt.Errorf(
			"%w: fixture CA and one to three explicit HTTPS targets required",
			errLoadSession,
		)
	}
	owned := make(map[string]string, len(targets))
	for host, destination := range targets {
		name, port, err := net.SplitHostPort(host)
		if err != nil || name == "" || !validTransportPort(port) {
			return nil, fmt.Errorf(
				"%w: fixture target requires hostname and explicit port",
				errLoadSession,
			)
		}
		address, destinationPort, err := net.SplitHostPort(destination)
		if err != nil || !validTransportPort(destinationPort) {
			return nil, fmt.Errorf(
				"%w: fixture TCP destination requires explicit loopback address and port",
				errLoadSession,
			)
		}
		ip := net.ParseIP(address)
		if ip == nil || !ip.IsLoopback() {
			return nil, fmt.Errorf(
				"%w: fixture TCP destination must be a literal loopback address",
				errLoadSession,
			)
		}
		owned[host] = destination
	}
	verified := &VerifiedTransport{targets: owned}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	verified.transport = &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			destination, ok := owned[address]
			if !ok {
				return nil, fmt.Errorf("%w: undeclared fixture TCP destination", errLoadSession)
			}
			return dialer.DialContext(ctx, network, destination)
		},
		TLSClientConfig:     &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        4096,
		MaxIdleConnsPerHost: 2048,
		MaxConnsPerHost:     2048,
		IdleConnTimeout:     90 * time.Second,
	}
	// Go may replay a bodyless POST bearing Idempotency-Key when a reused
	// connection loses its response, even without GetBody. Each unsafe request
	// gets a fresh connection; exact headers/body/key are preserved. The real
	// TLS overhead remains inside the write latency distribution.
	verified.commands = verified.transport.Clone()
	verified.commands.DisableKeepAlives = true
	return verified, nil
}

func validTransportPort(raw string) bool {
	port, err := strconv.Atoi(raw)
	return err == nil && port > 0 && port <= 65535 && strconv.Itoa(port) == raw
}

func (t *VerifiedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme != "https" || request.URL.User != nil {
		return nil, fmt.Errorf(
			"%w: load driver requires credential-free verified HTTPS URL",
			errLoadSession,
		)
	}
	if _, ok := t.targets[request.URL.Host]; !ok {
		return nil, fmt.Errorf("%w: load driver refused undeclared HTTPS target", errLoadSession)
	}
	// An alternate Host header must not bypass the exact front-door routing
	// contract while keeping SNI pointed at an allowed certificate.
	if request.Host != "" && request.Host != request.URL.Host {
		return nil, fmt.Errorf("%w: load driver refused alternate Host header", errLoadSession)
	}
	transport := t.transport
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		transport = t.commands
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		return nil, fmt.Errorf("verified fixture HTTPS: %w", err)
	}
	return response, nil
}

func (t *VerifiedTransport) CloseIdleConnections() {
	t.transport.CloseIdleConnections()
	t.commands.CloseIdleConnections()
}
