package scorecards

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestObservationEvidenceUsesItsNativeCurrentOwnerPolicy(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	notes := observations.Service{Pool: f.pool, Now: f.s.Now}
	id := uuid.New()
	page, err := notes.Change(
		ctx,
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		observations.Command{
			Action: "observation",
			ID:     id,
			Evidence: observations.Evidence{
				Category: "technical",
				Evidence: "UNREADABLE-NATIVE-OBSERVATION-CANARY",
			},
		},
	)
	if err != nil || len(page.Observations) != 1 {
		t.Fatal(err)
	}
	reviewer := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')",
		f.event,
		reviewer.ID,
	)
	f.exec(
		"INSERT INTO resource_assignments(event_id,resource_id,person_id) VALUES($1,$2,$3)",
		f.event,
		f.entry,
		reviewer.ID,
	)
	if _, err = notes.Read(
		ctx,
		reviewer.ID,
		f.event,
		f.team,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("fixture accidentally grants native note access", err)
	}
	command := f.command(f.checkpoint, "submit", true)
	version := int64(1)
	command.Scores[0].Evidence = []EvidenceReference{
		{Kind: "observation", ID: &id, Version: &version},
	}
	if _, err = f.s.Change(
		ctx,
		reviewer.ID,
		f.event,
		f.team,
		f.revision(),
		command,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("scorecard reference bypassed native private observation policy", err)
	}
	// A real assigned second mentor may reference the exact native version.
	if _, err = f.s.Change(ctx, f.second.ID, f.event, f.team, f.revision(), command); err != nil {
		t.Fatal("authorised native note reference denied", err)
	}
}

func TestSignoffObservationEvidenceRechecksEachAuthorsNativeGrant(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	noteID := uuid.New()
	notes := observations.Service{Pool: f.pool, Now: f.s.Now}
	if _, err := notes.Change(
		ctx,
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		observations.Command{
			Action: "observation",
			ID:     noteID,
			Evidence: observations.Evidence{
				Category: "technical",
				Evidence: "Synthetic owned final evidence",
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	final := f.configure("mentor_final")
	f.now = f.config.SubmissionCutoff
	f.freeze()
	command := f.command(final, "submit", true)
	version := int64(1)
	command.Scores[0].Evidence = []EvidenceReference{
		{Kind: "observation", ID: &noteID, Version: &version},
	}
	card, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), command)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	refs := []CardReference{card.Reference}
	proof := ModerationContext{
		ID:                 id,
		Version:            1,
		Binding:            "synthetic-final-note-owner",
		TeamID:             f.team,
		RunID:              card.RunID,
		SubmissionRevision: 1,
		RequiredAuthorIDs:  []uuid.UUID{f.primary.ID},
		Cards:              refs,
	}
	f.exec(
		"CREATE TABLE synthetic_moderation_contexts(event_id uuid,team_id uuid,id uuid,actor_id uuid,active boolean,document jsonb)",
	)
	raw, _ := json.Marshal(proof)
	f.exec(
		"INSERT INTO synthetic_moderation_contexts VALUES($1,$2,$3,$4,true,$5)",
		f.event,
		f.team,
		id,
		f.owner.ID,
		raw,
	)
	f.s.Moderation = recordedOwner{}
	f.exec(
		"UPDATE mentor_assignments SET snapshot='draft' WHERE event_id=$1 AND person_id=$2",
		f.event,
		f.primary.ID,
	)
	f.exec(
		"INSERT INTO resource_assignments(event_id,resource_id,person_id) VALUES($1,$2,$3)",
		f.event,
		f.entry,
		f.primary.ID,
	)
	// The author still has generic assessment authority and the moderator may
	// inspect the native note. Neither substitutes for the author's lost99 grant.
	if _, err = notes.Read(ctx, f.owner.ID, f.event, f.team); err != nil {
		t.Fatal(err)
	}
	err = database.Within(ctx, f.pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := f.s.guard(ctx, q, f.primary.ID, f.event, f.team); err != nil {
			t.Fatal("fixture lost generic assessment authority", err)
		}
		_, err := f.s.SignOffInTransaction(ctx, tx, f.owner.ID, f.event, f.team, id, refs)
		return err
	})
	if !errors.Is(err, access.ErrForbidden) {
		t.Fatal("moderator/native generic authority replaced each author's observation grant", err)
	}
}
