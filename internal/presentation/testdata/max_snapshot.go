//go:build ignore

// Regenerate from the repository root:
// go run internal/presentation/testdata/max_snapshot.go > web/e2e/fixtures/max-display-frame.sse
package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
	"github.com/google/uuid"
)

func main() {
	id := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	cue := presentation.DisplayCue{ID: id, Title: strings.Repeat("<", 200), Text: strings.Repeat("<", 4000), URL: "https://example.com/" + strings.Repeat("&", 2048-len("https://example.com/")), ContentTitle: strings.Repeat("<", 200)}
	snapshot := presentation.Snapshot{EventID: id, EventName: strings.Repeat("<", 160), Timezone: "Europe/London", Sequence: 7, ServerNow: time.Date(2030, 6, 14, 9, 0, 0, 0, time.UTC), Current: &cue, Next: &cue, Mode: "held", RemainingSeconds: 120, PrimaryColour: "#173a35", BackgroundColour: "#f8f9f5", ForegroundColour: "#173a35", Assets: []presentation.DisplayAsset{}}
	data, err := json.Marshal(snapshot)
	if err != nil {
		panic(err)
	}
	fmt.Printf("id: 7\nevent: snapshot\ndata: %s\n\n", data)
}
