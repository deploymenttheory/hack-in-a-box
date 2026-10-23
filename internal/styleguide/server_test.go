package styleguide_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/styleguide"
)

func TestStaticGuideNeedsNoApplicationAuthorityOrServices(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{"patterns.html": []byte("<!doctype html><title>Synthetic task patterns</title>"), "samples.html": []byte("<!doctype html><title>Synthetic component examples</title>"), "index.html": []byte("<!doctype html><title>Synthetic component guide</title>"), "assets/guide-hash.js": []byte("export const synthetic = true"), "assets/InterVariable-hash.woff2": []byte("wOF2synthetic-font")} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	secret := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(secret, []byte("PRIVATE-OUTSIDE-STATIC-ROOT"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "assets", "escape.js")); err != nil {
		t.Fatal(err)
	}
	server, err := styleguide.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/", 200},
		{"HEAD", "/", 200},
		{"GET", "/index.html", 200},
		{"GET", "/samples.html", 200},
		{"GET", "/patterns.html", 200},
		{"GET", "/assets/guide-hash.js", 200},
		{"GET", "/assets/InterVariable-hash.woff2", 200},
		{"GET", "/healthz", 200},
		{"GET", "/readyz", 200},
		{"GET", "/assets/", 404},
		{"GET", "/assets/escape.js", 404},
		{"GET", "/api/v1/session", 404},
		{"GET", "/auth/login", 404},
		{"GET", "/ops", 404},
		{"GET", "/../private.txt", 404},
		{"GET", "/source.ts", 404},
		{"POST", "/", 405},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, nil)
			request.Header.Set("Cookie", "hackbox_admin_session=PRIVATE-SESSION-CANARY")
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status %d, expected %d", response.Code, tc.status)
			}
			if len(response.Result().Cookies()) != 0 ||
				strings.Contains(response.Body.String(), "PRIVATE-") {
				t.Fatal("static guide touched/reflected application authority or outside files")
			}
			if tc.method == http.MethodHead && response.Body.Len() != 0 {
				t.Fatal("HEAD returned a body")
			}
			if !strings.Contains(
				response.Header().Get("Content-Security-Policy"),
				"connect-src 'none'",
			) {
				t.Fatal("guide may connect to APIs")
			}
			if tc.status == 200 && strings.HasPrefix(tc.path, "/assets/") {
				if !strings.Contains(response.Header().Get("Cache-Control"), "immutable") {
					t.Fatal("fingerprinted asset missing immutable cache")
				}
			} else if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("guide document/error cached")
			}
			if strings.HasSuffix(tc.path, ".woff2") &&
				(!bytes.HasPrefix(response.Body.Bytes(), []byte("wOF2")) || response.Header().Get("Content-Type") != "font/woff2") {
				t.Fatal("font bytes/MIME invalid")
			}
		})
	}
}

func TestReadinessNeedsOnlyTheStaticEntry(t *testing.T) {
	root := t.TempDir()
	server, err := styleguide.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	for _, tc := range []struct {
		path   string
		status int
	}{{"/healthz", 200}, {"/readyz", 503}} {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest("GET", tc.path, nil))
		if response.Code != tc.status {
			t.Fatalf("%s = %d", tc.path, response.Code)
		}
	}
	if err := os.WriteFile(
		filepath.Join(root, "index.html"),
		[]byte("<title>Synthetic guide</title>"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest("GET", "/readyz", nil))
	if response.Code != 503 {
		t.Fatal("Missing example entry incorrectly passed readiness")
	}
	if err := os.WriteFile(
		filepath.Join(root, "samples.html"),
		[]byte("<title>Synthetic samples</title>"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest("GET", "/readyz", nil))
	if response.Code != 503 {
		t.Fatal("Missing task entry incorrectly passed readiness")
	}
	if err := os.WriteFile(
		filepath.Join(root, "patterns.html"),
		[]byte("<title>Synthetic patterns</title>"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest("GET", "/readyz", nil))
	if response.Code != 200 {
		t.Fatal("Static entry did not make guide ready")
	}
}
