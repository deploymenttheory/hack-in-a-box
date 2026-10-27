package app

import (
	"errors"
	"net/http"
)

// This process-local transport exists only in the compiled test entrypoint.
// Native GitHub clients still perform their real OAuth/JWT/token/REST work;
// only the synthetic provider's network destination changes. It does not
// prove GitHub's production TLS, credentials, permissions or live sandbox.
type lifecycleProviderTransport struct {
	base *http.Transport
}

func newLifecycleProviderTransport() (*lifecycleProviderTransport, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("isolated lifecycle requires the standard HTTP transport")
	}
	base = base.Clone()
	// No host proxy or real provider account belongs to the synthetic network.
	base.Proxy = nil
	return &lifecycleProviderTransport{base: base}, nil
}

func (t *lifecycleProviderTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL == nil || request.URL.User != nil {
		return nil, errors.New("invalid isolated provider destination")
	}
	if request.URL.Scheme == "http" {
		var destination string
		switch request.URL.Host {
		case "public.localhost:8080":
			destination = "public:8080"
		case "admin.localhost:8080":
			destination = "admin:8080"
		case "provider.localhost:8080":
			destination = "provider:8080"
		case "styleguide.localhost:8080":
			destination = "styleguide:8080"
		}
		if destination != "" {
			copy := request.Clone(request.Context())
			target := *request.URL
			target.Host = destination
			copy.URL, copy.Host = &target, request.URL.Host
			return t.base.RoundTrip(copy)
		}
	}
	if request.URL.Scheme != "https" {
		return nil, errors.New("destination is outside the isolated lifecycle network")
	}
	var prefix string
	switch request.URL.Host {
	case "api.github.com":
		prefix = "/github-api"
	case "github.com":
		prefix = "/github"
	default:
		return nil, errors.New("destination is outside the isolated lifecycle provider allowlist")
	}
	copy := request.Clone(request.Context())
	target := *request.URL
	target.Scheme, target.Host = "http", "provider:8080"
	target.Path = prefix + target.Path
	if target.RawPath != "" {
		target.RawPath = prefix + target.RawPath
	}
	copy.URL, copy.Host = &target, "provider:8080"
	return t.base.RoundTrip(copy)
}
