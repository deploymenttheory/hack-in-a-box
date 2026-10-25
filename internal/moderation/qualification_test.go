package moderation

import (
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/jackc/pgx/v5"
)

func TestQualificationConsumesOnlyCanonicalCurrentSignedFinalSource(t *testing.T) {
	f, s, h := finalContext(t, true)
	h = f.change(s, h, f.owner.ID, Command{Action: "sign_off", Reason: "Complete native final evidence for qualification"})
	err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		source, err := s.QualificationSourceInTransaction(t.Context(), tx, f.event, f.team, h.Checkpoint.Definition.RunID)
		if err != nil {
			return err
		}
		if source.Signoff.ID != *h.SignoffID || source.HuddleVersion != h.Version || source.ModeratorID != f.owner.ID || source.SubmissionRevision != *h.SubmissionRevision {
			t.Fatal("qualification lost exact canonical final source", source)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
