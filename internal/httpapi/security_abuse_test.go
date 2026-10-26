package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurity121RequestFailuresDoNotLogCredentialsOrUntrustedMetadata(t *testing.T) {
	for _, failure := range []string{"panic", "error"} {
		t.Run(failure, func(t *testing.T) {
			router, logs := testRouter(t)
			router.Post("/security121", func(w http.ResponseWriter, r *http.Request) {
				if failure == "panic" {
					panic("PRIVATE-PANIC-PAYLOAD")
				}
				respondError(w, r, errors.New("PRIVATE-PROVIDER-ERROR"))
			})
			r := httptest.NewRequest(http.MethodPost, "/security121?secret=PRIVATE-QUERY-SECRET", strings.NewReader("PRIVATE-BODY"))
			r.Header.Set("Authorization", "Bearer PRIVATE-BEARER")
			r.Header.Set("Cookie", "hackbox_session=PRIVATE-COOKIE")
			r.Header.Set("X-CSRF-Token", "PRIVATE-CSRF")
			r.Header.Set("X-Request-ID", "PRIVATE-REQUEST-ID")
			// The router fixture has a deliberately tiny request limit: use an
			// empty body for the handler error, and test oversized body separately.
			r.Body = http.NoBody
			r.ContentLength = 0
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != http.StatusInternalServerError {
				t.Fatal("failure positive control did not reach handler")
			}
			if strings.Contains(w.Body.String()+logs.String()+w.Header().Get("X-Request-ID"), "PRIVATE-") {
				t.Fatal("request failure disclosed untrusted or confidential metadata")
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("failure response lost privacy headers")
			}
		})
	}
}
