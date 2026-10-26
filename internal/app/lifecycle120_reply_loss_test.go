package app

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var errLifecycleLostReply = errors.New("controlled synthetic acknowledgement reply loss")

// Only a completed real HTTP200 on this exact native POST is consumed. The
// caller receives no acknowledgement and must recover its original intent.
// This test-process transport never changes requests, auth, clocks or owners.
type lifecycleLostReplyTransport struct {
	base    http.RoundTripper
	target  string
	posts   atomic.Int64
	dropped atomic.Bool
}

func (l *lifecycleLostReplyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	match := request.Method == http.MethodPost && request.URL.String() == l.target
	if match {
		l.posts.Add(1)
	}
	response, err := l.base.RoundTrip(request)
	if err != nil {
		return response, err
	}
	if match && response.StatusCode == http.StatusOK && l.dropped.CompareAndSwap(false, true) {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		return nil, errLifecycleLostReply
	}
	return response, nil
}

func TestLifecycleControlledLostReplyKeepsOneHTTPWrite(t *testing.T) {
	var effects atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil || string(body) != "original-fixture-intent" || r.Header.Get("X-CSRF-Token") != "fixture-csrf" {
				t.Error("original HTTP write changed")
			}
			effects.Add(1)
		}
		_, _ = io.WriteString(w, "original-fixture-acknowledgement")
	}))
	defer server.Close()
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	defer base.CloseIdleConnections()
	loss := &lifecycleLostReplyTransport{base: base, target: server.URL + "/ack"}
	client := &http.Client{Transport: loss, Timeout: 3 * time.Second}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, loss.target, strings.NewReader("original-fixture-intent"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-CSRF-Token", "fixture-csrf")
	if response, err := client.Do(request); !errors.Is(err, errLifecycleLostReply) || response != nil {
		t.Fatal("accepted reply was not lost to the caller")
	}
	response, err := client.Get(loss.target)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "original-fixture-acknowledgement" || effects.Load() != 1 || loss.posts.Load() != 1 || !loss.dropped.Load() {
		t.Fatal("read recovery replayed or changed the original HTTP effect")
	}
}
