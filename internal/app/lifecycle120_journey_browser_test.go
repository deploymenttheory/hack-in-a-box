package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

// Browser barriers are private fixture IPC, never application endpoints or
// authentication overrides. The native test independently reads every effect.
type lifecycleBrowserPhase struct {
	Sequence int       `json:"sequence"`
	Action   string    `json:"action"`
	EventID  uuid.UUID `json:"event_id"`
	Now      time.Time `json:"domain_now"`
	Primary  string    `json:"primary"`
}

type lifecycleBrowserAcknowledgement struct {
	Sequence int       `json:"sequence"`
	Action   string    `json:"action"`
	EventID  uuid.UUID `json:"event_id"`
	Saved    bool      `json:"saved_by_ui"`
}

func lifecycleBrowserRequired(t *testing.T) bool {
	t.Helper()
	switch os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") {
	case "":
		return false
	case "planning":
		return true
	default:
		t.Fatal("unknown native browser phase selection")
		return false
	}
}

func (r *lifecycleJourney) saveBrandInBrowser(t *testing.T, item *lifecycleJourneyEvent, sequence int) {
	t.Helper()
	var clock struct {
		Now time.Time `json:"now"`
	}
	if err := r.control.call(t.Context(), http.MethodGet, "/clock", nil, &clock); err != nil {
		t.Fatal(err)
	}
	phase := lifecycleBrowserPhase{
		Sequence: sequence, Action: "save-branding", EventID: item.view.ID,
		Now: clock.Now, Primary: item.brand.Primary,
	}
	directory := "/artifacts/browser"
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(phase)
	if err != nil {
		t.Fatal(err)
	}
	request := filepath.Join(directory, fmt.Sprintf("phase-%d.json", sequence))
	if err := os.WriteFile(request+".tmp", encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(request+".tmp", request); err != nil {
		t.Fatal(err)
	}
	response := filepath.Join(directory, fmt.Sprintf("result-%d.json", sequence))
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	for {
		body, readErr := os.ReadFile(response)
		if readErr == nil {
			var ack lifecycleBrowserAcknowledgement
			if err := json.Unmarshal(body, &ack); err != nil {
				t.Fatal("invalid browser phase receipt")
			}
			if ack.Sequence != sequence || ack.EventID != item.view.ID || ack.Action != phase.Action || !ack.Saved {
				t.Fatal("browser phase did not acknowledge the exact native event action")
			}
			break
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			t.Fatal(readErr)
		}
		select {
		case <-t.Context().Done():
			t.Fatal("native journey cancelled while awaiting browser action")
		case <-deadline.C:
			t.Fatal("native browser branding action did not finish within its bounded barrier")
		case <-ticker.C:
		}
	}
	var native templates.BrandView
	if err := r.owner.call(t.Context(), http.MethodGet,
		"/api/v1/events/"+item.view.ID.String()+"/branding", -1, nil, &native, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(native.Branding, item.brand) || native.Revision != item.view.Revision+1 {
		t.Fatal("browser save differs from independently observed native branding/revision")
	}
	item.browserBrandVerified = true
}
