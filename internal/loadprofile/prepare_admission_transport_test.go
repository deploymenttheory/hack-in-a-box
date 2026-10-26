package loadprofile

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

// A controlled real HTTPS transport only, not native domain/authentication
// proof. Lost original responses retain the captured command/revision tuple;
// later caller state cannot rewrite it or authorize a repeated domain write.
func TestPreparationLostOriginalResponseRetainsCapturedIdentifiers(t *testing.T) {
	var effects atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.Header.Get("If-Match") != `"9"` {
			t.Error("original captured request changed")
		}
		effects.Add(1)
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = connection.Close()
	}))
	defer server.Close()
	session := &Session{
		origin: server.URL,
		client: server.Client(),
		person: uuid.New(),
		csrf:   "synthetic-csrf",
	}
	preparer := &NativePreparer{}
	revision := int64(9)
	id, request, command := uuid.New(), uuid.New(), uuid.New()
	input := map[string]any{"id": id, "request_id": request, "command_id": command}
	if _, err := prepareCall[map[string]any](
		t.Context(),
		preparer,
		session,
		http.MethodPut,
		"/api/v1/outcomes/"+uuid.NewString(),
		&revision,
		input,
	); err == nil {
		t.Fatal("lost response became acknowledged")
	}
	revision = 10
	input["id"] = uuid.New()
	input["request_id"] = uuid.New()
	input["command_id"] = uuid.New()
	if effects.Load() != 1 || len(preparer.Calls) != 1 {
		t.Fatal("uncertain write was replayed")
	}
	call := preparer.Calls[0]
	if call.OriginalComplete || call.Status != 0 || call.ExpectedRevision == nil ||
		*call.ExpectedRevision != 9 ||
		call.PayloadID == nil ||
		*call.PayloadID != id ||
		call.RequestID == nil ||
		*call.RequestID != request ||
		call.InputSHA256 == "" || call.CommandID == nil || *call.CommandID != command {
		t.Fatal(
			"original uncertainty or exact identifiers were replaced by later caller state",
			call,
		)
	}
}
