package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
)

func TestResponseContractConcurrentValidationRemainsStrict(t *testing.T) {
	for i := range 16 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			routes, err := responseContractRouter()
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"/healthz", "/readyz"} {
				request := httptest.NewRequest(http.MethodGet, path, nil)
				route, params, err := routes.FindRoute(request)
				if err != nil {
					t.Fatal(err)
				}
				for _, tc := range []struct {
					body   string
					status int
					valid  bool
				}{{`{"status":"ok"}`, 200, true}, {`{"status":"unknown"}`, 200, false}, {`{}`, 200, false}, {`{"status":"ok"}`, 503, false}} {
					input := &openapi3filter.ResponseValidationInput{
						RequestValidationInput: &openapi3filter.RequestValidationInput{
							Request:    request,
							PathParams: params,
							Route:      route,
						},
						Status: tc.status,
						Header: http.Header{
							"Content-Type": []string{"application/json"},
						},
						Body: io.NopCloser(strings.NewReader(tc.body)),
					}
					err := openapi3filter.ValidateResponse(t.Context(), input)
					if (err == nil) != tc.valid {
						t.Fatalf("%s status%d body%s validation=%v", path, tc.status, tc.body, err)
					}
				}
			}
			for _, request := range []*http.Request{httptest.NewRequest(http.MethodPost, "/healthz", nil), httptest.NewRequest(http.MethodGet, "/outside-contract", nil)} {
				if _, _, err := routes.FindRoute(request); err == nil {
					t.Fatal("shared response router accepted an unknown method/path")
				}
			}
		})
	}
}
