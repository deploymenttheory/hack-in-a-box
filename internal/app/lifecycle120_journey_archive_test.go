package app

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/archives"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
)

type lifecycleCompetitionDigest struct {
	Rows   int64  `json:"rows"`
	SHA256 string `json:"sha256"`
}

type lifecycleArchiveReceipt struct {
	Competition           map[string]lifecycleCompetitionDigest `json:"competition"`
	SummaryAvailable      bool                                  `json:"organiser_summary_available"`
	ParticipantRestricted bool                                  `json:"participant_summary_restricted"`
	AuditedOutcomeVersion int64                                 `json:"audited_outcome_version"`
	Unchanged             bool                                  `json:"competition_unchanged_after_archive_and_followup"`
}

// The fixture's only SQL access here is a repeatable-read, read-only oracle.
// Native HTTP creates every lifecycle/evidence effect. Full private rows exist
// only in memory for hashing and are never logged or put into public receipts.
func lifecycleCompetitionSnapshot(
	t *testing.T,
	eventID uuid.UUID,
	expectedSignoffs int,
) map[string]lifecycleCompetitionDigest {
	t.Helper()
	if os.Getenv("HACKBOX_LIFECYCLE_ISOLATED") != "synthetic-only" {
		t.Fatal("competition oracle requires the isolated fixture")
	}
	pool, err := database.Open(t.Context(), os.Getenv("HACKBOX_DATABASE_URL"))
	if err != nil {
		t.Fatal("open isolated competition oracle")
	}
	defer pool.Close()
	tx, err := pool.BeginTx(
		t.Context(),
		pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
	)
	if err != nil {
		t.Fatal("begin read-only competition oracle")
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	// Fixed schema identifiers, never client-provided SQL. Preserve full rows,
	// including private document fields; sorting canonical row JSON gives a stable
	// independent comparison without relying on the API's changing capabilities.
	sources := map[string]string{}
	for _, table := range []string{"submission_states", "submission_revisions", "submission_freeze_sources", "rubric_versions", "rubric_approvals", "rubric_publications", "mentor_scorecard_versions", "mentor_scorecard_signoffs", "judge_cards", "judge_history", "judge_result_approvals", "result_versions", "result_publications"} {
		sources[table] = "SELECT to_jsonb(s)::text AS row FROM " + table + " s WHERE s.event_id=$1"
	}
	sources["submission_freeze_decisions"] = "SELECT to_jsonb(d)::text AS row FROM submission_freeze_decisions d JOIN submission_freeze_sources s ON s.id=d.source_id WHERE s.event_id=$1"
	out := map[string]lifecycleCompetitionDigest{}
	for name, source := range sources {
		var raw string
		var count int64
		if err := tx.QueryRow(t.Context(), "SELECT count(*),COALESCE(jsonb_agg(row ORDER BY row),'[]'::jsonb)::text FROM ("+source+") native_rows", eventID).
			Scan(&count, &raw); err != nil {
			t.Fatal("read isolated competition source: " + name)
		}
		if name == "mentor_scorecard_signoffs" {
			// All-team judging has no final mentor huddles. Qualification's four
			// acknowledged huddles each have an immutable sign-off; retain and hash
			// the empty all-team set rather than inventing a qualification gate.
			if count != int64(expectedSignoffs) {
				t.Fatalf(
					"competition sign-off census differs from acknowledged final huddles: got %d, want %d",
					count,
					expectedSignoffs,
				)
			}
		} else if count == 0 {
			t.Fatal("competition oracle unexpectedly has no native rows: " + name)
		}
		hash := sha256.Sum256([]byte(raw))
		out[name] = lifecycleCompetitionDigest{Rows: count, SHA256: hex.EncodeToString(hash[:])}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal("finish read-only competition oracle")
	}
	return out
}

func (r *lifecycleJourney) archiveCompetitionAndUpdateOutcome(t *testing.T) {
	t.Helper()
	for i := range r.events {
		item := &r.events[i]
		var err error
		item.view, err = lifecycleTransition(
			t.Context(),
			r.owner,
			item.view.ID,
			"archived",
			http.StatusOK,
		)
		if err != nil {
			t.Fatal(err)
		}
		if item.view.Stage != "archived" {
			t.Fatal("native archive transition did not take effect")
		}
		base := "/api/v1/events/" + item.view.ID.String()
		var archive archives.View
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			base+"/archive",
			-1,
			nil,
			&archive,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if !archive.ReadOnly || archive.Summary == nil || archive.SummaryState != "available" ||
			archive.CompletedAt == nil {
			t.Fatal("organiser archive lacks actual retained event evidence")
		}
		own := r.participant(t, *item.outcomes[0].OwnerID)
		if err := own.call(
			t.Context(),
			http.MethodGet,
			base+"/archive",
			-1,
			nil,
			&archive,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if !archive.ReadOnly || archive.Summary != nil || archive.SummaryState != "restricted" {
			t.Fatal("participant archive disclosed restricted staff summary")
		}
		current := &item.outcomes[0]
		before := current.Version.Version
		command := outcomes.Command{
			ID:     uuid.New(),
			Action: "evidence",
			Reason: "Authorised retained follow-up after competition archive",
			Evidence: lifecycleOutcomeEvidence(
				"self_report",
				"Synthetic owner progress after archive; not independently demonstrated adoption",
				"",
				nil,
			),
		}
		path := "/api/v1/outcomes/" + current.ID.String()
		if err := own.call(
			t.Context(),
			http.MethodPut,
			path,
			before,
			command,
			current,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if current.Version.Version != before+1 || current.CommandID != command.ID ||
			!current.Capability.Archived {
			t.Fatal("archived follow-up lost its exact native acknowledgement")
		}
		var history outcomes.History
		if err := own.call(
			t.Context(),
			http.MethodGet,
			path+"/history",
			-1,
			nil,
			&history,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, version := range history.Items {
			if version.CommandID == command.ID && version.Version == current.Version.Version &&
				version.ActorID == own.person &&
				version.EvidenceAddedID != nil &&
				*version.EvidenceAddedID == command.Evidence.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("archived outcome update lacks exact retained actor/evidence history")
		}
		after := lifecycleCompetitionSnapshot(t, item.view.ID, len(item.review.FinalHuddles))
		if !reflect.DeepEqual(item.competitionBefore, after) {
			t.Fatal("archive or authorised outcomes mutated frozen competition evidence")
		}
		item.archive = lifecycleArchiveReceipt{
			Competition:           after,
			SummaryAvailable:      true,
			ParticipantRestricted: true,
			AuditedOutcomeVersion: current.Version.Version,
			Unchanged:             true,
		}
	}
}
