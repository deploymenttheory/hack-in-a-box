package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

type lifecycleIntentMetadata struct {
	ObservedAt       time.Time            `json:"observed_at"`
	ActorID          uuid.UUID            `json:"actor_id"`
	Method           string               `json:"method"`
	Path             string               `json:"path"`
	ExpectedRevision int64                `json:"expected_revision"`
	IdempotencyKey   uuid.UUID            `json:"idempotency_key,omitempty"`
	Identifiers      map[string]uuid.UUID `json:"identifiers"`
}

func TestLifecycleIntentJournalFailurePreventsWireAttempt(t *testing.T) {
	t.Setenv("HACKBOX_LIFECYCLE_ISOLATED", "synthetic-only")
	t.Setenv("HACKBOX_LIFECYCLE_INTENT_JOURNAL", filepath.Join(t.TempDir(), "absent", "intent.jsonl"))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client := lifecycleClient{origin: server.URL, http: server.Client(), person: uuid.New(), csrf: "PRIVATE_CSRF_CANARY"}
	err := client.call(t.Context(), http.MethodPost, "/api/v1/example", 0, map[string]any{"request_id": uuid.New(), "reason": "PRIVATE_REASON_CANARY"}, nil, http.StatusOK)
	if err == nil || requests.Load() != 0 {
		t.Fatal("mutation reached the wire without a retained original identity")
	}
	if strings.Contains(err.Error(), "CANARY") {
		t.Fatal("journal failure exposed private input")
	}
}

func lifecycleIntentMetadataFor(actor uuid.UUID, method, path string, revision int64, payload []byte, key uuid.UUID) lifecycleIntentMetadata {
	route, _ := url.Parse(path)
	metadata := lifecycleIntentMetadata{ObservedAt: time.Now().UTC(), ActorID: actor, Method: method, ExpectedRevision: revision, IdempotencyKey: key, Identifiers: map[string]uuid.UUID{}}
	if route != nil {
		metadata.Path = route.EscapedPath()
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) == nil {
		for _, name := range []string{"id", "request_id", "command_id"} {
			var value string
			if json.Unmarshal(fields[name], &value) != nil {
				continue
			}
			if id, err := uuid.Parse(value); err == nil && id != uuid.Nil {
				metadata.Identifiers[name] = id
			}
		}
	}
	return metadata
}

// Retain original caller identity before the wire attempt, so a timeout can be
// reconciled by its owning native acknowledgement route without guessing or
// resending it. Only positive-projection metadata enters this private journal.
func retainLifecycleIntent(actor uuid.UUID, method, path string, revision int64, payload []byte, key uuid.UUID) error {
	name := os.Getenv("HACKBOX_LIFECYCLE_INTENT_JOURNAL")
	if name == "" || method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return nil
	}
	if os.Getenv("HACKBOX_LIFECYCLE_ISOLATED") != "synthetic-only" {
		return errors.New("intent journal requires isolated synthetic execution")
	}
	metadata := lifecycleIntentMetadataFor(actor, method, path, revision, payload, key)
	raw, err := json.Marshal(metadata)
	if err != nil {
		return errors.New("encode private lifecycle intent metadata before sending")
	}
	file, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return errors.New("retain original lifecycle intent before sending")
	}
	_, writeErr := file.Write(append(raw, '\n'))
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return errors.New("finish retaining original lifecycle intent before sending")
	}
	return nil
}

func TestLifecycleIntentMetadataNeverCopiesPayloadHeadersOrQuery(t *testing.T) {
	id, key := uuid.New(), uuid.New()
	payload := []byte(`{"request_id":"` + id.String() + `","command_id":"not-an-identity","private_key":"PRIVATE_KEY_CANARY","reason":"PRIVATE_PROSE_CANARY","body":{"request_id":"NESTED_BODY_CANARY"},"access_token":"TOKEN_CANARY"}`)
	metadata := lifecycleIntentMetadataFor(uuid.New(), http.MethodPost, "/api/v1/events/one/results?token=QUERY_CANARY", 7, payload, key)
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"PRIVATE_KEY_CANARY", "PRIVATE_PROSE_CANARY", "NESTED_BODY_CANARY", "TOKEN_CANARY", "QUERY_CANARY"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("private data copied into intent metadata")
		}
	}
	if metadata.Path != "/api/v1/events/one/results" || metadata.ExpectedRevision != 7 || metadata.Identifiers["request_id"] != id || len(metadata.Identifiers) != 1 || metadata.IdempotencyKey != key {
		t.Fatal("original safe caller identity was not preserved")
	}
}
