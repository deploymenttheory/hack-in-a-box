package qualification

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestNativeURLAndFileFinalsAreNotApplicableWhileMissingWithdrawnInactiveAndDisbandedAreUnscored(
	t *testing.T,
) {
	for _, kind := range []string{"url", "file"} {
		t.Run(kind, func(t *testing.T) { qualificationNonManagedOutcomes(t, kind) })
	}
}

func qualificationNonManagedOutcomes(t *testing.T, kind string) {
	f := setupPool(t, testdb.New(t, true), 5)
	cp := f.configure("mentor_final")
	deliverable := uuid.New()
	defs := templates.EmptyDefinitions()
	defs.Deliverables = []templates.Deliverable{
		{
			ID:            deliverable,
			Title:         "Synthetic hosted build",
			Required:      true,
			EvidenceKinds: []string{kind},
			Audience:      []string{"mentor", "judge", "organiser"},
		},
	}
	raw, _ := json.Marshal(defs)
	if err := dbgen.New(f.pool).
		SaveEventDefinitions(t.Context(), dbgen.SaveEventDefinitionsParams{EventID: f.event, Definitions: raw}); err != nil {
		t.Fatal(err)
	}
	native := submissions.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	f.now = f.config.LiveStart
	source, err := native.Read(t.Context(), f.member.ID, f.event, f.team)
	if err != nil {
		t.Fatal(err)
	}
	evidence := submissions.Evidence{
		DeliverableID: deliverable,
		Kind:          kind,
		Label:         "Exact formal hosted build",
		URL:           "https://synthetic.invalid/formal-build",
	}
	if kind == "file" {
		storage, openErr := files.OpenLocal(t.TempDir())
		if openErr != nil {
			t.Fatal(openErr)
		}
		t.Cleanup(func() {
			if closeErr := storage.Close(); closeErr != nil {
				t.Error(closeErr)
			}
		})
		owner := files.Service{
			Pool:    f.pool,
			Storage: storage,
			Policy: access.EventPolicy{
				Queries: dbgen.New(f.pool),
				Clock:   func() time.Time { return f.now },
			},
			MaxBytes:   1000,
			EventQuota: 10000,
		}
		file, uploadErr := owner.Upload(
			t.Context(),
			files.Upload{
				Actor:     f.member.ID,
				Event:     f.event,
				TeamID:    &f.team,
				Key:       uuid.New(),
				Revision:  f.revision(),
				Filename:  "synthetic-final.txt",
				MediaType: "text/plain",
				Content:   []byte("Synthetic protected final evidence"),
			},
		)
		if uploadErr != nil {
			t.Fatal(uploadErr)
		}
		evidence.URL = ""
		evidence.FileID = &file.ID
	}
	if _, err = native.Submit(
		t.Context(),
		f.member.ID,
		f.event,
		f.team,
		source.Revision,
		submissions.Command{Evidence: []submissions.Evidence{evidence}},
	); err != nil {
		t.Fatal(err)
	}
	f.now = f.config.SubmissionCutoff
	if _, err = native.Read(t.Context(), f.owner.ID, f.event, f.team); err != nil {
		t.Fatal(err)
	}
	huddles := f.huddles()
	nativeFinalProof(f, cp, huddles)
	// Actual native outcome API; repository/team lifecycle facts remain fixtures.
	withdrawn := f.teams[2]
	source, err = native.Read(t.Context(), f.owner.ID, f.event, withdrawn.team)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = native.Outcome(
		t.Context(),
		f.owner.ID,
		f.event,
		withdrawn.team,
		source.Revision,
		"withdrawn",
		"Explicit synthetic team withdrawal",
	); err != nil {
		t.Fatal(err)
	}
	f.exec(
		"UPDATE competition_entries SET active=false WHERE event_id=$1 AND team_id=$2",
		f.event,
		f.teams[3].team,
	)
	f.exec(
		"UPDATE competition_entries SET active=false WHERE event_id=$1 AND team_id=$2",
		f.event,
		f.teams[4].team,
	)
	f.exec(
		"UPDATE hack_teams SET state='disbanded' WHERE event_id=$1 AND id=$2",
		f.event,
		f.teams[4].team,
	)
	f.exec(
		"INSERT INTO team_repositories(event_id,team_id,organization_id,organization,name,app_id,installation_id,template_repository,requested_by,state)VALUES($1,$2,99,'synthetic-org','unused-disbanded',42,55,'template',$3,'pending')",
		f.event,
		f.teams[4].team,
		f.owner.ID,
	)
	s := Service{
		Pool:       f.pool,
		Now:        func() time.Time { return f.now },
		Cards:      &f.s,
		Moderation: huddles,
		Freeze:     &submissionfreeze.Service{Pool: f.pool, Now: func() time.Time { return f.now }},
	}
	p, err := s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		cp.Definition.RunID,
		f.revision(),
		panelCommand(f),
	)
	if err != nil {
		t.Fatal(err)
	}
	p = reviewedCard(f, s, cp.Definition.RunID, p, 7)
	eligible := 0
	expected := []string{"frozen", "not_submitted", "withdrawn", "withdrawn", "disbanded"}
	for index, team := range f.teams {
		at := slices.IndexFunc(
			p.Teams,
			func(status TeamStatus) bool { return status.TeamID == team.team },
		)
		if at < 0 {
			t.Fatal("missing truthful team outcome", team.team)
		}
		status := p.Teams[at]
		if status.Outcome != expected[index] {
			t.Fatal("outcome was conflated with unresolved code", status, expected[index])
		}
		if index == 0 {
			if !status.Eligible || status.FreezeStatus != "not_applicable" ||
				status.CanDecideEligibility {
				t.Fatal("formal URL became missing managed code", status)
			}
			eligible++
		} else if status.Eligible || status.CanSaveCard || status.CanSignCard || status.CanDecideEligibility || status.Source != nil || len(status.Blockers) != 0 {
			t.Fatal("excluded team entered scoring or unresolved-code workflow", status)
		}
	}
	if eligible != 1 || len(p.Round.Cards) != 1 || p.Round.Cards[0].TeamID != f.teams[0].team ||
		p.Preview == nil ||
		len(p.Preview.Rankings) != 1 ||
		p.Preview.Rankings[0].TeamID != f.teams[0].team {
		t.Fatal("noncompeting outcomes influenced qualification", p)
	}
	var operations int
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM operations WHERE event_id=$1 AND target_id=$2", f.event, f.teams[4].team).
		Scan(&operations); err != nil ||
		operations != 0 {
		t.Fatal("unused disbanded repository scheduled work", operations, err)
	}
}
