package app

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
	"strings"
	"time"

	"github.com/google/uuid"
)

// Each client signs in through the real OIDC callback and owns a separate cookie
// jar. A second client signs the same person into the independent other portal;
// no scenario code writes sessions, people or competition facts into PostgreSQL.
type lifecycleClient struct {
	origin string
	http   *http.Client
	person uuid.UUID
	csrf   string
	name   string
}

func signInLifecycle(ctx context.Context, origin, issuer, subject string) (*lifecycleClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	c := &lifecycleClient{origin: origin, http: &http.Client{
		Jar: jar, Timeout: 15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}
	redirect, err := c.redirect(ctx, origin+"/auth/login?return_to=%2Fevents", http.StatusFound)
	if err != nil {
		return nil, fmt.Errorf("start normal OIDC login: %w", err)
	}
	authorize, err := url.Parse(redirect)
	if err != nil || authorize.Scheme+"://"+authorize.Host+authorize.Path != issuer+"/authorize" {
		return nil, errors.New("application did not redirect to the configured synthetic issuer")
	}
	query := authorize.Query()
	query.Set("actor", subject)
	authorize.RawQuery = query.Encode()
	callback, err := c.redirect(ctx, authorize.String(), http.StatusFound)
	if err != nil {
		return nil, fmt.Errorf("synthetic provider authorization: %w", err)
	}
	target, err := url.Parse(callback)
	if err != nil || target.Scheme+"://"+target.Host+target.Path != origin+"/auth/callback" {
		return nil, errors.New("provider callback did not match the exact portal")
	}
	if _, err = c.redirect(ctx, callback, http.StatusSeeOther); err != nil {
		return nil, fmt.Errorf("normal OIDC callback: %w", err)
	}
	var session struct {
		Person struct {
			ID          uuid.UUID `json:"id"`
			DisplayName string    `json:"display_name"`
		} `json:"person"`
		CSRF string `json:"csrf_token"`
	}
	if err = c.call(ctx, http.MethodGet, "/api/v1/session", -1, nil, &session, http.StatusOK); err != nil {
		return nil, fmt.Errorf("read native authenticated identity: %w", err)
	}
	if session.Person.ID == uuid.Nil || len(session.CSRF) < 20 {
		return nil, errors.New("normal OIDC login produced no native person or CSRF context")
	}
	c.person, c.csrf, c.name = session.Person.ID, session.CSRF, session.Person.DisplayName
	return c, nil
}

func (c *lifecycleClient) redirect(ctx context.Context, target string, want int) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != want {
		return "", fmt.Errorf("expected normal authorization redirect, status %d", response.StatusCode)
	}
	location, err := response.Location()
	if err != nil {
		return "", errors.New("normal authorization redirect had no valid location")
	}
	return location.String(), nil
}

func (c *lifecycleClient) call(
	ctx context.Context,
	method, path string,
	revision int64,
	input, output any,
	want int,
) error {
	return c.callIntent(ctx, method, path, revision, input, output, want, uuid.Nil)
}

// Only APIs that require a caller-owned Idempotency-Key use this explicit
// intent. Default callers keep their existing headers and transport behavior.
func (c *lifecycleClient) callIntent(
	ctx context.Context,
	method, path string,
	revision int64,
	input, output any,
	want int,
	commandID uuid.UUID,
) error {
	if !strings.HasPrefix(path, "/api/v1/") || strings.HasPrefix(path, "//") {
		return errors.New("lifecycle commands must target a native API route")
	}
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.origin+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Origin", c.origin)
	request.Header.Set("Cache-Control", "no-store")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && method != http.MethodHead {
		request.Header.Set("X-CSRF-Token", c.csrf)
	}
	if revision >= 0 {
		request.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
	}
	if commandID != uuid.Nil {
		request.Header.Set("Idempotency-Key", commandID.String())
		// A keyed write must still return uncertainty to this caller after an
		// accepted response is lost, rather than replaying the body automatically.
		request.GetBody = nil
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return err
	}
	if response.StatusCode != want {
		// Do not copy session tokens, credentials or private domain prose from an
		// unexpected response into the release evidence/logs.
		return &lifecycleStatusError{Method: method, Actual: response.StatusCode, Expected: want}
	}
	if !strings.Contains(response.Header.Get("Cache-Control"), "no-store") {
		return errors.New("native API response did not retain no-store")
	}
	if output != nil {
		if err = json.Unmarshal(raw, output); err != nil {
			return fmt.Errorf("decode native API response: %w", err)
		}
	}
	return nil
}

type lifecycleStatusError struct {
	Method           string
	Actual, Expected int
}

func (e *lifecycleStatusError) Error() string {
	return fmt.Sprintf("%s native API returned %d, expected %d", e.Method, e.Actual, e.Expected)
}
