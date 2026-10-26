package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"time"
)

type lifecycleControl struct {
	http *http.Client
}

func newLifecycleControl(directory string) (*lifecycleControl, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("private lifecycle socket directory must be absolute")
	}
	socket := filepath.Join(directory, "control.sock")
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
			if address != "lifecycle-control:80" {
				return nil, errors.New("fixture control cannot dial a network endpoint")
			}
			var dial net.Dialer
			return dial.DialContext(ctx, "unix", socket)
		},
	}
	return &lifecycleControl{http: &http.Client{
		Transport:     transport,
		Timeout:       10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (c *lifecycleControl) close() { c.http.CloseIdleConnections() }

// Distinct real commands must also be chronologically distinct in the fixture.
// This advances only domain time; auth, leases, provider TTLs and timers stay real.
func (c *lifecycleControl) nextAction(ctx context.Context) error {
	var phase struct {
		Now time.Time `json:"now"`
	}
	if err := c.call(ctx, http.MethodGet, "/clock", nil, &phase); err != nil {
		return err
	}
	if phase.Now.IsZero() {
		return errors.New("fixture action has no current domain instant")
	}
	return c.advance(ctx, phase.Now.Add(time.Second))
}

func (c *lifecycleControl) initialTimeline(ctx context.Context) (lifecycleTimeline, error) {
	var phase struct {
		Now      time.Time         `json:"now"`
		Timeline lifecycleTimeline `json:"timeline"`
	}
	if err := c.call(ctx, http.MethodGet, "/clock", nil, &phase); err != nil {
		return phase.Timeline, err
	}
	if phase.Timeline.Initial.IsZero() || !phase.Now.Equal(phase.Timeline.Initial) ||
		!phase.Timeline.Initial.Before(
			phase.Timeline.Publication,
		) || !phase.Timeline.Publication.Before(phase.Timeline.Live) {
		return phase.Timeline, errors.New(
			"checkpoint requires the provider's fresh initial domain phase",
		)
	}
	return phase.Timeline, nil
}

func (c *lifecycleControl) call(ctx context.Context, method, path string, input, output any) error {
	switch path {
	case "/clock", "/mail/fault", "/mail/messages", "/github/proof", "/github/branch-fault":
	default:
		return errors.New("unknown owner-private lifecycle control")
	}
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://lifecycle-control"+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return errors.New("owner-private lifecycle control unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" {
		return fmt.Errorf(
			"owner-private lifecycle control returned status %d without a valid response",
			response.StatusCode,
		)
	}
	if output == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(output); err != nil {
		return errors.New("invalid owner-private lifecycle control response")
	}
	return nil
}

func (c *lifecycleControl) advance(ctx context.Context, at time.Time) error {
	var phase struct {
		Now time.Time `json:"now"`
	}
	if err := c.call(
		ctx,
		http.MethodPost,
		"/clock",
		map[string]time.Time{"at": at},
		&phase,
	); err != nil {
		return err
	}
	if !phase.Now.Equal(at) {
		return errors.New("fixture did not reach the exact requested domain phase")
	}
	return nil
}
