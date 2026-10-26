package teamsworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

// Assert the network boundary, not only the URL parser. A secret-bearing URL
// rejected by either layer must not reach a dialer or enter an error receipt.
func TestSecurity121SSRFTransportRejectsDestinationsBeforeDial(t *testing.T) {
	for name, raw := range map[string]string{
		"metadata":         "http://169.254.169.254/latest/meta-data/",
		"loopback":         "https://127.0.0.1/private",
		"ipv6_loopback":    "https://[::1]/private",
		"decimal_loopback": "https://2130706433/private",
		"encoded_loopback": "https://%31%32%37.0.0.1/private",
		"userinfo":         strings.Replace(endpoint, "https://", "https://synthetic:PRIVATE-CREDENTIAL@", 1),
		"suffix_confusion": strings.Replace(endpoint, testHost, testHost+".attacker.invalid", 1),
		"fragment":         endpoint + "#PRIVATE-FRAGMENT",
		"query_injection":  endpoint + "&url=http%3A%2F%2F169.254.169.254",
	} {
		t.Run(name, func(t *testing.T) {
			lookups, dials := 0, 0
			a := &adapter{lookup: lookupFunc(func(context.Context, string, string) ([]netip.Addr, error) {
				lookups++
				return nil, errors.New("PRIVATE-DNS-DETAIL")
			}), dial: func(context.Context, string, string) (net.Conn, error) {
				dials++
				return nil, errors.New("PRIVATE-DIAL-DETAIL")
			}}
			got := a.send(t.Context(), raw, []byte(`{"text":"synthetic"}`))
			if lookups != 0 || dials != 0 || got.Outcome != jobs.Permanent || got.Reason != "unsafe_configuration" {
				t.Fatal("unsafe URL crossed the transport boundary", name)
			}
			assertSecurity121SafeResult(t, got)
		})
	}
	for name, addresses := range map[string][]string{
		"loopback": {"127.0.0.1"}, "metadata": {"169.254.169.254"},
		"private_v4": {"10.0.0.1"}, "private_v6": {"fd00::1"},
		"mapped_v4": {"::ffff:127.0.0.1"}, "mixed_last_private": {"20.50.1.2", "169.254.169.254"},
		"mixed_first_private": {"127.0.0.1", "20.50.1.2"},
	} {
		t.Run("dns_"+name, func(t *testing.T) {
			dials, lookups := 0, 0
			a := &adapter{lookup: lookupFunc(func(_ context.Context, network, host string) ([]netip.Addr, error) {
				lookups++
				if network != "ip" || host != testHost {
					t.Error("lookup escaped configured provider host")
				}
				out := []netip.Addr{}
				for _, address := range addresses {
					out = append(out, netip.MustParseAddr(address))
				}
				return out, nil
			}), dial: func(context.Context, string, string) (net.Conn, error) {
				dials++
				return nil, errors.New("PRIVATE-DIAL-DETAIL")
			}}
			got := a.send(t.Context(), endpoint, []byte(`{"text":"synthetic"}`))
			if lookups != 1 || dials != 0 || got.Outcome != jobs.Permanent || got.Reason != "unsafe_destination" {
				t.Fatal("DNS answer admitted an internal destination", name)
			}
			assertSecurity121SafeResult(t, got)
		})
	}
	// The positive public-address control attempts exactly the resolved IP,
	// never resolves a second time after a DNS rebinding opportunity. A dial
	// error remains retryable/sanitised; it does not expose the credential URL.
	lookups, dials := 0, 0
	a := &adapter{lookup: lookupFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		lookups++
		return []netip.Addr{netip.MustParseAddr("20.50.1.2")}, nil
	}), dial: func(_ context.Context, network, address string) (net.Conn, error) {
		dials++
		if network != "tcp" || address != "20.50.1.2:443" {
			t.Error("transport did not pin resolved public address")
		}
		return nil, errors.New("PRIVATE-DIAL-DETAIL")
	}}
	got := a.send(t.Context(), endpoint, []byte(`{"text":"synthetic"}`))
	if lookups != 1 || dials != 1 || got.Outcome != jobs.Retryable || got.Reason != "connection_unavailable" {
		t.Fatal("positive public-address control did not exercise transport")
	}
	assertSecurity121SafeResult(t, got)
}

func assertSecurity121SafeResult(t *testing.T, result result) {
	t.Helper()
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"PRIVATE-", "SYNTHETIC_SIGNATURE", "powerplatform.com", "169.254.169.254"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("network receipt contained confidential destination/provider detail")
		}
	}
}
