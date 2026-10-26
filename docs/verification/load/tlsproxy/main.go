// The load fixture's external TLS terminator. It does not authenticate users,
// create application facts or change the normal app/worker clock.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type proxyConfig struct{ listen, host, upstream, certificate, key string }

func (c proxyConfig) validate() (*url.URL, string, error) {
	front, err := url.Parse("https://" + c.host)
	if err != nil || front.Host != c.host || front.Hostname() == "" || front.Port() == "" ||
		front.User != nil || front.Path != "" || front.RawQuery != "" || front.Fragment != "" || net.ParseIP(front.Hostname()) != nil {
		return nil, "", errors.New("an exact named frontend host and port are required")
	}
	upstream, err := url.Parse(c.upstream)
	if err != nil || upstream.Scheme != "http" || upstream.Hostname() == "" || upstream.Port() == "" ||
		upstream.User != nil || (upstream.Path != "" && upstream.Path != "/") || upstream.RawQuery != "" || upstream.Fragment != "" {
		return nil, "", errors.New("one explicit private HTTP upstream host and port are required")
	}
	return upstream, front.Hostname(), nil
}

type proxyTransport struct{ reads, writes *http.Transport }

func newProxyTransport() *proxyTransport {
	reads := http.DefaultTransport.(*http.Transport).Clone()
	reads.Proxy = nil // No ambient proxy may receive native cookies or OAuth codes.
	reads.DisableCompression = true
	reads.ResponseHeaderTimeout = 15 * time.Second
	reads.MaxIdleConns, reads.MaxIdleConnsPerHost = 2048, 1024
	writes := reads.Clone()
	// On a fresh HTTP/1 upstream connection Transport cannot replay an
	// already-processed write after a lost response, even for a bodyless
	// request carrying the native Idempotency-Key. The measured baseline
	// includes this private connection cost; no provider time is subtracted.
	writes.DisableKeepAlives = true
	return &proxyTransport{reads: reads, writes: writes}
}

func (p *proxyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	switch request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return p.reads.RoundTrip(request)
	default:
		// A native Idempotency-Key is a receipt identity, not permission to resend.
		request.GetBody = nil
		return p.writes.RoundTrip(request)
	}
}

func (p *proxyTransport) close() { p.reads.CloseIdleConnections(); p.writes.CloseIdleConnections() }

func newProxy(c proxyConfig) (http.Handler, func(), error) {
	target, name, err := c.validate()
	if err != nil {
		return nil, nil, err
	}
	transport := newProxyTransport()
	proxy := &httputil.ReverseProxy{
		Transport:     transport,
		ErrorLog:      log.New(io.Discard, "", 0),
		FlushInterval: -1, // Forward SSE frames while the upstream stream stays open.
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(target)
			request.Out.Host = request.In.Host
			request.Out.Header.Del("Forwarded")
			request.SetXForwarded() // ReverseProxy first removes untrusted X-Forwarded-*.
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			// Do not put authenticated URLs, codes, bodies or transport errors in logs.
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		},
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || !strings.EqualFold(r.Host, c.host) || !strings.EqualFold(r.TLS.ServerName, name) {
			http.Error(w, "misdirected request", http.StatusMisdirectedRequest)
			return
		}
		proxy.ServeHTTP(w, r)
	})
	return handler, transport.close, nil
}

func run(ctx context.Context, args []string) error {
	var c proxyConfig
	flags := flag.NewFlagSet("load-tls-proxy", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&c.listen, "listen", ":8443", "TLS listener")
	flags.StringVar(&c.host, "host", "", "Exact external DNS host:port")
	flags.StringVar(&c.upstream, "upstream", "", "Private HTTP service URL")
	flags.StringVar(&c.certificate, "cert", "", "TLS certificate PEM")
	flags.StringVar(&c.key, "key", "", "TLS private key PEM")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("invalid proxy arguments")
	}
	handler, closeIdle, err := newProxy(c)
	if err != nil {
		return err
	}
	defer closeIdle()
	certificate, err := tls.LoadX509KeyPair(c.certificate, c.key)
	if err != nil {
		return errors.New("TLS certificate/key could not be loaded")
	}
	server := &http.Server{Addr: c.listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, IdleTimeout: 90 * time.Second,
		TLSConfig:   &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}},
		BaseContext: func(net.Listener) context.Context { return ctx }, ErrorLog: log.New(io.Discard, "", 0)}
	stopped := make(chan error, 1)
	go func() { stopped <- server.ListenAndServeTLS("", "") }()
	select {
	case err := <-stopped:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("TLS listener stopped unexpectedly")
	case <-ctx.Done():
		deadline, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(deadline); err != nil {
			_ = server.Close()
			return errors.New("TLS listener did not drain")
		}
		if err := <-stopped; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return errors.New("TLS listener stopped unexpectedly")
		}
		return nil
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
