package loadprofile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
)

var errLoadSession = errors.New("invalid load session")

const NativeRequestTimeout = 15 * time.Second

// Session owns one genuine application-issued cookie jar. Credentials and
// response bodies are intentionally private and never part of JSON reports.
type Session struct {
	origin  string
	client  *http.Client
	person  uuid.UUID
	csrf    string
	surface Surface
}

func (s *Session) Person() uuid.UUID { return s.person }
func (s *Session) Close()            { s.client.CloseIdleConnections() }

// SignIn uses the ordinary login/state/PKCE/callback/session paths. The only
// synthetic behavior is choosing an actor at the configured fixture issuer;
// neither sessions nor people are inserted directly into PostgreSQL.
// A caller supplies a normal verified-TLS transport trusted for its fixture CA.
func SignIn(
	ctx context.Context,
	surface Surface,
	origin, issuer, subject string,
	transport http.RoundTripper,
) (*Session, error) {
	if err := validateSignInOrigins(surface, origin, issuer); err != nil {
		return nil, err
	}
	if subject == "" {
		return nil, fmt.Errorf("%w: synthetic OIDC actor is required", errLoadSession)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create native session cookie jar: %w", err)
	}
	s := &Session{
		origin:  origin,
		surface: surface,
		client: &http.Client{
			Jar:           jar,
			Transport:     transport,
			Timeout:       NativeRequestTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	redirect := func(target string, status int) (string, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return "", fmt.Errorf("%w: invalid normal OIDC request", errLoadSession)
		}
		response, err := s.client.Do(request)
		if err != nil {
			return "", fmt.Errorf("%w: normal OIDC transport failed", errLoadSession)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != status {
			return "", fmt.Errorf(
				"%w: normal OIDC redirect returned status %d",
				errLoadSession,
				response.StatusCode,
			)
		}
		location, err := response.Location()
		if err != nil {
			return "", fmt.Errorf("%w: normal OIDC redirect location absent", errLoadSession)
		}
		return location.String(), nil
	}
	authorize, err := redirect(origin+"/auth/login?return_to=%2Fevents", http.StatusFound)
	if err != nil {
		return nil, err
	}
	target, err := url.Parse(authorize)
	if err != nil || target.Scheme+"://"+target.Host+target.Path != issuer+"/authorize" ||
		target.User != nil {
		return nil, fmt.Errorf(
			"%w: normal login did not select the configured synthetic issuer",
			errLoadSession,
		)
	}
	query := target.Query()
	query.Set("actor", subject)
	target.RawQuery = query.Encode()
	callback, err := redirect(target.String(), http.StatusFound)
	if err != nil {
		return nil, err
	}
	target, err = url.Parse(callback)
	if err != nil || target.Scheme+"://"+target.Host+target.Path != origin+"/auth/callback" ||
		target.User != nil {
		return nil, fmt.Errorf(
			"%w: synthetic provider did not select the exact application callback",
			errLoadSession,
		)
	}
	if _, err = redirect(callback, http.StatusSeeOther); err != nil {
		return nil, err
	}
	response, err := s.Exchange(ctx, http.MethodGet, "/api/v1/session", nil, nil)
	if err != nil {
		return nil, err
	}
	var current struct {
		Person struct {
			ID uuid.UUID `json:"id"`
		} `json:"person"`
		CSRF string `json:"csrf_token"`
	}
	if response.Status != http.StatusOK || json.Unmarshal(response.Body, &current) != nil ||
		current.Person.ID == uuid.Nil ||
		len(current.CSRF) < 20 {
		return nil, fmt.Errorf(
			"%w: native OIDC callback did not establish a current person and CSRF session",
			errLoadSession,
		)
	}
	s.person, s.csrf = current.Person.ID, current.CSRF
	return s, nil
}

type Response struct {
	Status   int
	Duration time.Duration
	Body     []byte
	Received time.Time
}

// Exchange makes one native request. There is no write retry or inferred ACK.
// The complete body-read latency is measured, including TLS/network/server
// time. Provider-facing operations must remain separately labelled, rather
// than subtracting arbitrary provider latency from this measurement.
func (s *Session) Exchange(
	ctx context.Context,
	method, relative string,
	revision *int64,
	input any,
) (Response, error) {
	return s.exchange(ctx, method, relative, revision, input, uuid.Nil)
}

// Command preserves a caller-supplied native idempotency key. It performs one
// request only; a missing response is not permission to generate another key.
func (s *Session) Command(
	ctx context.Context,
	method, relative string,
	revision *int64,
	input any,
	key uuid.UUID,
) (Response, error) {
	if key == uuid.Nil || (method != http.MethodPost && method != http.MethodPut) {
		return Response{}, fmt.Errorf(
			"%w: explicit command identity and mutating method required",
			errLoadSession,
		)
	}
	return s.exchange(ctx, method, relative, revision, input, key)
}

func (s *Session) exchange(
	ctx context.Context,
	method, relative string,
	revision *int64,
	input any,
	key uuid.UUID,
) (Response, error) {
	var result Response
	target, err := url.Parse(relative)
	if err != nil || target.IsAbs() || target.Host != "" || target.Fragment != "" ||
		!strings.HasPrefix(target.Path, "/api/v1/") ||
		path.Clean(target.Path) != target.Path {
		return result, fmt.Errorf(
			"%w: load request must target an exact native API path",
			errLoadSession,
		)
	}
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return result, fmt.Errorf("%w: load command JSON encoding failed", errLoadSession)
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, s.origin+relative, body)
	if err != nil {
		return result, fmt.Errorf("%w: load native request invalid", errLoadSession)
	}
	if method != http.MethodGet && method != http.MethodHead {
		// bytes.Reader normally supplies GetBody. Combined with the native
		// Idempotency-Key, that lets net/http replay a consumed POST whose
		// response was lost. Keep the real key, but never rewind a write body.
		request.GetBody = nil
	}
	request.Header.Set("Origin", s.origin)
	request.Header.Set("Cache-Control", "no-store")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && method != http.MethodHead {
		request.Header.Set("X-CSRF-Token", s.csrf)
	}
	if revision != nil {
		request.Header.Set("If-Match", fmt.Sprintf("\"%d\"", *revision))
	}
	if key != uuid.Nil {
		request.Header.Set("Idempotency-Key", key.String())
	}
	started := time.Now()
	response, err := s.client.Do(request)
	if err != nil {
		result.Duration = time.Since(started)
		return result, fmt.Errorf(
			"%w: native HTTP transport failed; write outcome is not inferred",
			errLoadSession,
		)
	}
	defer func() { _ = response.Body.Close() }()
	result.Status = response.StatusCode
	result.Body, err = io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	result.Received = time.Now()
	result.Duration = time.Since(started)
	if err != nil || len(result.Body) > 4<<20 {
		return result, fmt.Errorf(
			"%w: native response incomplete or exceeds bounded load capture",
			errLoadSession,
		)
	}
	if !strings.Contains(response.Header.Get("Cache-Control"), "no-store") {
		return result, fmt.Errorf("%w: native load response lacks no-store", errLoadSession)
	}
	return result, nil
}

func validateSignInOrigins(surface Surface, origin, issuer string) error {
	if surface != PublicSurface && surface != AdminSurface {
		return fmt.Errorf("%w: native person portal required", errLoadSession)
	}
	for _, value := range []string{origin, issuer} {
		u, err := url.Parse(value)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" ||
			u.RawQuery != "" ||
			u.Fragment != "" {
			return fmt.Errorf(
				"%w: load baseline requires exact HTTPS application and issuer origins",
				errLoadSession,
			)
		}
	}

	return nil
}
