// Package teamsworkflow owns scoped Teams Workflows broadcast delivery. Endpoint
// URLs are credentials: neither network errors nor receipts may contain them.
package teamsworkflow

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

var (
	hostLabel    = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	workflowPath = regexp.MustCompile(
		`^/powerautomate/automations/direct/workflows/(?:[a-fA-F0-9]{32}|[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12})/triggers/manual/paths/invoke$`,
	)
	signature = regexp.MustCompile(`^[A-Za-z0-9+/=_-]{16,512}$`)
)

// Current public-cloud Workflows use the environment Power Platform endpoint.
// Microsoft retired the prior logic.azure.com trigger URLs on 30 November2025.
// See docs/teams-workflows-setup.md for the primary sources and supported scope.
func validateEndpoint(raw string) (*url.URL, bool) {
	if len(raw) < 1 || len(raw) > 4096 || strings.TrimSpace(raw) != raw ||
		strings.Contains(raw, "#") {
		return nil, false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Opaque != "" ||
		(u.Port() != "" && u.Port() != "443") ||
		u.RawPath != "" {
		return nil, false
	}
	host := strings.ToLower(u.Hostname())
	const suffix = ".environment.api.powerplatform.com"
	if !strings.HasSuffix(host, suffix) {
		return nil, false
	}
	labels := strings.Split(strings.TrimSuffix(host, suffix), ".")
	if len(labels) < 1 || len(labels) > 2 {
		return nil, false
	}
	for _, label := range labels {
		if !hostLabel.MatchString(label) {
			return nil, false
		}
	}
	if !workflowPath.MatchString(u.Path) {
		return nil, false
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil || query.Get("api-version") != "1" || !signature.MatchString(query.Get("sig")) {
		return nil, false
	}
	for key, values := range query {
		if len(values) != 1 {
			return nil, false
		}
		switch key {
		case "api-version", "sig":
		case "sp":
			if values[0] != "/triggers/manual/run" {
				return nil, false
			}
		case "sv":
			if values[0] != "1.0" {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	u.Host = host
	return u, true
}

var forbiddenNetworks = func() []netip.Prefix {
	out := make([]netip.Prefix, 0, 29)
	for _, value := range []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "::/96", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "100:0:0:1::/64", "2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20", "5f00::/16", "fc00::/7", "fec0::/10", "fe80::/10", "ff00::/8"} {
		out = append(out, netip.MustParsePrefix(value))
	}
	return out
}()

func publicAddress(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Is4In6() || ip.Zone() != "" || !ip.IsGlobalUnicast() {
		return false
	}
	for _, prefix := range forbiddenNetworks {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

type resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}
type result struct {
	Outcome    jobs.Outcome
	Status     int
	Reason     string
	RetryAfter time.Duration
}

// adapter's resolver/dial/trust injection is private and used by isolated tests.
// Production construction never permits a proxy, redirect, alternate CA or DNS
// answer to bypass the destination policy.
type adapter struct {
	lookup resolver
	dial   func(context.Context, string, string) (net.Conn, error)
	roots  *x509.CertPool
}

func newAdapter() *adapter {
	return &adapter{
		lookup: net.DefaultResolver,
		dial:   (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
	}
}

func (a *adapter) send(ctx context.Context, endpoint string, payload []byte) result {
	u, valid := validateEndpoint(endpoint)
	if !valid {
		return result{Outcome: jobs.Permanent, Reason: "unsafe_configuration"}
	}
	if len(payload) > 28*1024 {
		return result{Outcome: jobs.Permanent, Reason: "payload_too_large"}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	addresses, err := a.lookup.LookupNetIP(ctx, "ip", u.Hostname())
	if err != nil || len(addresses) == 0 {
		return result{
			Outcome:    jobs.Retryable,
			Reason:     "dns_unavailable",
			RetryAfter: 2 * time.Second,
		}
	}
	for _, ip := range addresses {
		if !publicAddress(ip) {
			return result{Outcome: jobs.Permanent, Reason: "unsafe_destination"}
		}
	}
	pinned := net.JoinHostPort(addresses[0].String(), "443")
	transport := &http.Transport{
		Proxy:                 nil,
		DisableKeepAlives:     true,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: u.Hostname(),
			RootCAs:    a.roots,
		},
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return a.dial(ctx, "tcp", pinned) },
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	var connected atomic.Bool
	ctx = httptrace.WithClientTrace(
		ctx,
		&httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { connected.Store(true) }},
	)
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		u.String(),
		bytes.NewReader(payload),
	)
	if err != nil {
		return result{Outcome: jobs.Permanent, Reason: "unsafe_configuration"}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		if connected.Load() {
			return result{Outcome: jobs.Ambiguous, Reason: "uncertain_response"}
		}
		return result{
			Outcome:    jobs.Retryable,
			Reason:     "connection_unavailable",
			RetryAfter: 2 * time.Second,
		}
	}
	_ = response.Body.Close()
	status := response.StatusCode
	if status >= 200 && status < 300 {
		return result{Outcome: jobs.Delivered, Status: status, Reason: "provider_accepted"}
	}
	if status == 429 {
		delay := 2 * time.Second
		if value := response.Header.Get("Retry-After"); value != "" {
			if seconds, parseErr := strconv.ParseInt(value, 10, 32); parseErr == nil {
				if seconds < 0 || seconds > 900 {
					return result{
						Outcome: jobs.Permanent,
						Status:  status,
						Reason:  "rate_limit_requires_review",
					}
				}
				delay = time.Duration(seconds) * time.Second
			} else if instant, parseErr := http.ParseTime(value); parseErr == nil {
				delay = time.Until(instant)
			} else {
				return result{
					Outcome: jobs.Permanent,
					Status:  status,
					Reason:  "rate_limit_requires_review",
				}
			}
		}
		if delay < time.Second {
			delay = time.Second
		}
		if delay > 15*time.Minute {
			return result{
				Outcome: jobs.Permanent,
				Status:  status,
				Reason:  "rate_limit_requires_review",
			}
		}
		return result{
			Outcome:    jobs.Retryable,
			Status:     status,
			Reason:     "rate_limited",
			RetryAfter: delay,
		}
	}
	if status >= 500 || status == 408 {
		return result{Outcome: jobs.Ambiguous, Status: status, Reason: "uncertain_acceptance"}
	}
	if status >= 300 && status < 400 {
		return result{Outcome: jobs.Permanent, Status: status, Reason: "redirect_rejected"}
	}
	return result{Outcome: jobs.Permanent, Status: status, Reason: "provider_rejected"}
}
