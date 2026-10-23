package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubwebhooks"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

const githubWebhookPath = "/api/v1/integrations/github/webhook"

// A single machine endpoint bypasses browser sessions and CSRF. It is registered
// after generated routes to avoid JSON decoding/normalization before the HMAC.
// Its OpenAPI operation documents the same bounded raw-body contract.
func githubWebhookRoute(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == githubWebhookPath
}

func githubWebhookHandler(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.GitHubWebhooks == nil {
			writeError(w, r, 503, "unavailable", "Webhook processing is temporarily unavailable.")
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" || r.Header.Get("Content-Encoding") != "" {
			writeError(w, r, 415, "unsupported_media", "Send the original JSON webhook body.")
			return
		}
		for _, name := range []string{"X-GitHub-Delivery", "X-GitHub-Event", "X-Hub-Signature-256"} {
			if len(r.Header.Values(name)) != 1 {
				respondError(w, r, access.ErrInvalid)
				return
			}
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, githubwebhooks.MaxBodyBytes))
		if err != nil {
			var size *http.MaxBytesError
			if errors.As(err, &size) {
				writeError(w, r, 413, "body_too_large", "The webhook body exceeds the limit.")
			} else {
				respondError(w, r, access.ErrInvalid)
			}
			return
		}

		err = deps.GitHubWebhooks.Receive(
			r.Context(),
			githubwebhooks.Headers{
				DeliveryID: r.Header.Get("X-GitHub-Delivery"),
				Event:      r.Header.Get("X-GitHub-Event"),
				Signature:  r.Header.Get("X-Hub-Signature-256"),
			},
			raw,
		)
		if errors.Is(err, githubwebhooks.ErrCapacity) {
			w.Header().Set("Retry-After", "30")
			writeError(
				w,
				r,
				503,
				"webhook_capacity",
				"Webhook processing is temporarily at capacity.",
			)
			return
		}
		if err != nil {
			respondError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}
}

// Raw ingestion is deliberately owned by the HTTP adapter above. Generated
// parsing must never replace exact-byte signature authentication.
func (h handlers) IngestGitHubWebhook(
	context.Context,
	contract.IngestGitHubWebhookRequestObject,
) (contract.IngestGitHubWebhookResponseObject, error) {
	return nil, access.ErrInvalid
}

func (h handlers) GetGitHubRepositoryActivity(
	ctx context.Context,
	r contract.GetGitHubRepositoryActivityRequestObject,
) (contract.GetGitHubRepositoryActivityResponseObject, error) {
	if h.GitHubWebhooks == nil {
		return nil, access.ErrNotFound
	}
	status, err := h.GitHubWebhooks.Status(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read repository activity: %w", err)
	}
	return convertDTO[contract.GetGitHubRepositoryActivity200JSONResponse](status)
}
