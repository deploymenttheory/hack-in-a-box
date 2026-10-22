package rubric_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestRubricNotificationSourceRequiresActualCurrentPublicationAndCurrentAudience(t *testing.T) {
	f := publicationSetup(t)
	ref := f.configuration.Reference
	challenge := f.challenge(content.Page{}, &ref)
	var source rubric.NotificationSource
	check := func(wantVersion int64, oldMustDeny bool) {
		t.Helper()
		err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			items, err := f.rubric.DiscoverNotificationSourcesInTransaction(
				t.Context(),
				tx,
				f.eventID,
				&challenge.ID,
			)
			if err != nil {
				return err
			}
			if wantVersion == 0 {
				if len(items) != 0 {
					return fmt.Errorf("planned date invented actual rubric publication")
				}
				return nil
			}
			if len(items) != 1 || items[0].ChallengeVersion != wantVersion {
				return fmt.Errorf("wrong actual publication: %+v", items)
			}
			if oldMustDeny {
				if _, err = f.rubric.ResolveNotificationAudienceInTransaction(
					t.Context(),
					tx,
					f.eventID,
					source.ChallengeID,
					source.ChallengeVersion,
				); err == nil {
					return fmt.Errorf("superseded rubric source still dispatchable")
				}
			}
			source = items[0]
			people, err := f.rubric.ResolveNotificationAudienceInTransaction(
				t.Context(),
				tx,
				f.eventID,
				source.ChallengeID,
				source.ChallengeVersion,
			)
			if err != nil {
				return err
			}
			if len(people) != 2 || !slices.Contains(people, f.first.ID) ||
				!slices.Contains(people, f.second.ID) {
				return fmt.Errorf("current confirmed audience missing: %v", people)
			}
			raw, _ := json.Marshal(items)
			if strings.Contains(string(raw), "PRIVATE") {
				return fmt.Errorf("private judge data entered routing metadata")
			}
			wrong := uuid.New()
			unknown, err := f.rubric.DiscoverNotificationSourcesInTransaction(
				t.Context(),
				tx,
				f.eventID,
				&wrong,
			)
			if err != nil || len(unknown) != 0 {
				return fmt.Errorf("requested source filter ignored: %+v %v", unknown, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	f.now = f.due
	check(0, false)
	if err := f.content.Dispatch(
		t.Context(),
		f.eventID,
		challenge.ID,
		challenge.Revision,
	); err != nil {
		t.Fatal(err)
	}
	check(challenge.Version, false)
	definition := f.configuration.Definition
	definition.Categories[0].Weight = 2
	f.change(rubric.Command{Action: "save", Definition: &definition})
	f.change(rubric.Command{Action: "approve", Version: f.configuration.Reference.Version})
	released, err := f.content.Read(t.Context(), f.owner.ID, f.eventID, challenge.ID, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	next := f.configuration.Reference
	replacement := f.challenge(released, &next)
	// A future correction draft does not replace the actual public source.
	check(challenge.Version, false)
	f.change(
		rubric.Command{
			Action:       "review_impact",
			Version:      next.Version,
			ImpactReview: "PRIVATE-IMPACT-CANARY",
			Notice:       "Reviewed public correction",
		},
	)
	if _, err = f.content.Action(
		t.Context(),
		f.owner.ID,
		f.eventID,
		replacement.ID,
		replacement.Revision,
		"release",
		"Reviewed correction",
	); err != nil {
		t.Fatal(err)
	}
	check(replacement.Version, true)
	late := testdb.Person(t, f.pool)
	f.exec(
		`INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')`,
		f.eventID,
		late.ID,
	)
	f.exec(
		`UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2`,
		f.eventID,
		f.first.ID,
	)
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		people, e := f.rubric.ResolveNotificationAudienceInTransaction(
			t.Context(),
			tx,
			f.eventID,
			source.ChallengeID,
			source.ChallengeVersion,
		)
		if e != nil {
			return e
		}
		if len(people) != 1 || people[0] != f.second.ID {
			return fmt.Errorf("withdrawn recipient retained routing authority: %v", people)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := f.content.Read(t.Context(), f.owner.ID, f.eventID, challenge.ID, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	challengeOnly := f.challenge(current, &next)
	if _, err = f.content.Action(
		t.Context(),
		f.owner.ID,
		f.eventID,
		challengeOnly.ID,
		challengeOnly.Revision,
		"release",
		"Challenge-only clarification; criteria unchanged",
	); err != nil {
		t.Fatal(err)
	}
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		items, e := f.rubric.DiscoverNotificationSourcesInTransaction(
			t.Context(),
			tx,
			f.eventID,
			&challenge.ID,
		)
		if e != nil {
			return e
		}
		if len(items) != 0 {
			return fmt.Errorf("unchanged criteria fabricated another rubric notice: %+v", items)
		}
		if _, e = f.rubric.ResolveNotificationAudienceInTransaction(
			t.Context(),
			tx,
			f.eventID,
			source.ChallengeID,
			source.ChallengeVersion,
		); e == nil {
			return fmt.Errorf("old reviewed material notice became replayable")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
