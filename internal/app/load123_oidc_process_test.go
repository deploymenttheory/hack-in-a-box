package app

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/loadprofile"
)

// TestLoad123OIDCFixtureProcess is an EXTERNAL provider-only process compiled
// into the private verification image. Normal API/admin/worker images do not
// invoke it. It has no domain clock, native identity/session seed, application
// handler, GitHub runtime or administrative control route.
func TestLoad123OIDCFixtureProcess(t *testing.T) {
	if os.Getenv("HACKBOX_LOAD_OIDC_FIXTURE") != "1" {
		t.Skip("explicit private load issuer process only")
	}
	path := os.Getenv("HACKBOX_LOAD_OIDC_MANIFEST")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("private load issuer manifest unavailable")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		t.Fatal("private load issuer manifest must be a regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, loadprofile.MaximumOIDCManifestBytes+1))
	if err != nil {
		t.Fatal("private load issuer manifest read failed")
	}
	manifest, err := loadprofile.ParseOIDCFixtureManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	people := make(map[string]lifecyclePerson, len(manifest.People))
	for subject, person := range manifest.People {
		people[subject] = lifecyclePerson{Subject: person.Subject, Name: person.Name, Email: person.Email}
	}
	provider, err := newLifecycleOIDCForPeople(manifest.Issuer, manifest.Clients, people)
	if err != nil {
		t.Fatal("explicit synthetic load issuer construction failed")
	}
	address := os.Getenv("HACKBOX_LOAD_OIDC_LISTEN")
	if address == "" {
		address = ":8080"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal("synthetic load issuer listen failed")
	}
	server := &http.Server{Handler: provider, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 90 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	select {
	case err := <-finished:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatal("synthetic load issuer server failed")
		}
	case <-ctx.Done():
		bounded, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(bounded); err != nil {
			_ = server.Close()
			t.Fatal("synthetic load issuer did not drain")
		}
		if err := <-finished; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatal("synthetic load issuer stopped unexpectedly")
		}
	}
}
