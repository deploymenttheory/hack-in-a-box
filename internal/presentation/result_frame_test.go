package presentation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/resultpublication"
	"github.com/google/uuid"
)

// Codec sizing only: actual publication and paired-token authority are exercised
// by the native app consumer tests. These DTOs do not fabricate a release grant.
func TestMaximumAnnouncedResultsAndCuesFitTheBoundedRoomFrame(t *testing.T) {
	now := time.Date(2030, 6, 14, 9, 0, 0, 0, time.UTC)
	cue := DisplayCue{ID: uuid.New(), Title: strings.Repeat("<", 200), Text: strings.Repeat("<", 4000), URL: "https://example.com/" + strings.Repeat("&", 2048-len("https://example.com/")), ContentTitle: strings.Repeat("<", 200)}
	finalists := resultpublication.Announcement{ID: uuid.New(), Kind: "finalists", Version: 1, Finalists: []resultpublication.NamedTeam{}, Reserves: []resultpublication.NamedTeam{}, Awards: []resultpublication.AnnouncedAward{}, PublishedAt: now}
	for range 100 {
		finalists.Finalists = append(finalists.Finalists, resultpublication.NamedTeam{ID: uuid.New(), Name: strings.Repeat("<", 160)})
		finalists.Reserves = append(finalists.Reserves, resultpublication.NamedTeam{ID: uuid.New(), Name: strings.Repeat("<", 160)})
	}
	awards := resultpublication.Announcement{ID: uuid.New(), Kind: "awards", Version: 2, Finalists: []resultpublication.NamedTeam{}, Reserves: []resultpublication.NamedTeam{}, Awards: []resultpublication.AnnouncedAward{}, PublishedAt: now}
	category := uuid.New()
	for range 50 {
		awards.Awards = append(awards.Awards, resultpublication.AnnouncedAward{PrizeID: uuid.New(), Name: strings.Repeat("<", 160), CategoryID: &category, Team: finalists.Finalists[0], Amount: strings.Repeat("9", 64), Currency: "GBP"})
	}
	snapshot := Snapshot{EventID: uuid.New(), EventName: strings.Repeat("<", 160), Timezone: "Europe/London", Sequence: 7, ServerNow: now, Current: &cue, Next: &cue, Mode: "held", RemainingSeconds: 120, PrimaryColour: "#173a35", BackgroundColour: "#f8f9f5", ForegroundColour: "#173a35", Assets: []DisplayAsset{}, Results: []resultpublication.Announcement{finalists, awards}}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	// Include the SSE id/event/data envelope, beyond the encoded JSON body.
	frameSize := len(raw) + len("id: 7\nevent: snapshot\ndata: \n\n")
	if frameSize <= 128*1024 || frameSize >= 512*1024 {
		t.Fatal("legal escaped result frame no longer fits the measured transport budget", frameSize)
	}
	t.Logf("actual Go-escaped maximum result/cue frame: %d bytes", frameSize)
}
