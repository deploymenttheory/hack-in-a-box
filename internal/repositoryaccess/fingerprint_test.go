package repositoryaccess

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
)

func TestRepositoryCommandFingerprintBrowserParity(t *testing.T) {
	encoded, err := os.ReadFile("../../api/repository-access-fingerprint-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Input struct {
			Action       string    `json:"action"`
			ActorID      uuid.UUID `json:"actorID"`
			EventID      uuid.UUID `json:"eventID"`
			TeamID       uuid.UUID `json:"teamID"`
			RepositoryID int64     `json:"repositoryID"`
			PlanID       uuid.UUID `json:"planID"`
			Version      int64     `json:"version"`
			Reason       string    `json:"reason"`
		} `json:"input"`
		Expected string `json:"expected"`
	}
	if err = json.Unmarshal(encoded, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, vector := range vectors {
		in := vector.Input
		plan := Plan{ID: in.PlanID, EventID: in.EventID, TeamID: in.TeamID, RepositoryID: in.RepositoryID, Version: in.Version, Reason: in.Reason, ApprovalReason: in.Reason}
		if got := commandFingerprint(in.Action, in.ActorID, plan); got != vector.Expected {
			t.Fatalf("%s command identity differs from shared browser vector: got %s want %s", in.Action, got, vector.Expected)
		}
	}
}
