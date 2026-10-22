package submissions_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	t                                             *testing.T
	pool                                          *pgxpool.Pool
	q                                             *dbgen.Queries
	s                                             submissions.Service
	owner, member, other, mentor, judge           dbgen.Person
	id, team, otherTeam, repo, attachment, secret uuid.UUID
	now                                           time.Time
	c                                             event.Configuration
	defs                                          templates.Definitions
}

func newFixture(t *testing.T, pool *pgxpool.Pool) *fixture {
	t.Helper()
	f := &fixture{
		t:          t,
		pool:       pool,
		q:          dbgen.New(pool),
		owner:      testdb.Person(t, pool),
		member:     testdb.Person(t, pool),
		other:      testdb.Person(t, pool),
		mentor:     testdb.Person(t, pool),
		judge:      testdb.Person(t, pool),
		team:       uuid.New(),
		otherTeam:  uuid.New(),
		repo:       uuid.New(),
		attachment: uuid.New(),
		secret:     uuid.New(),
	}
	f.id = testdb.Event(t, pool, f.owner.ID)
	f.c = event.DefaultConfiguration()
	f.c.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	f.c.LiveEnd = f.c.LiveStart.Add(8 * time.Hour)
	f.c.SubmissionCutoff = f.c.LiveEnd.Add(-time.Hour)
	f.c.TeamFreeze = f.c.LiveStart.Add(-8 * 24 * time.Hour)
	f.c.AdmissionCutoff = f.c.TeamFreeze.Add(-time.Hour)
	f.now = f.c.LiveStart
	f.s = submissions.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	f.defs = templates.EmptyDefinitions()
	f.defs.Deliverables = []templates.Deliverable{
		{ID: f.repo, Title: "Repository", Required: true, EvidenceKinds: []string{"repository"}},
		{ID: f.attachment, Title: "Technical notes", EvidenceKinds: []string{"file", "url"}},
		{
			ID:            f.secret,
			Title:         "Organiser-only evidence",
			EvidenceKinds: []string{"file"},
			Audience:      []string{"organiser"},
		},
	}
	f.saveConfig()
	f.saveDefs()
	f.exec(
		"UPDATE events SET stage='preparation',published_at=$2,team_frozen_at=$3 WHERE id=$1",
		f.id,
		f.c.LiveStart.Add(-7*24*time.Hour),
		f.c.TeamFreeze,
	)
	for _, pair := range []struct{ team, person uuid.UUID }{{f.team, f.member.ID}, {f.otherTeam, f.other.ID}} {
		f.exec(
			"INSERT INTO hack_teams(id,event_id,name,state,frozen,captain_id,published_captain_id) VALUES($1,$2,'Synthetic team','published',true,$3,$3)",
			pair.team,
			f.id,
			pair.person,
		)
		if err := f.q.CreateCompetitionEntry(
			t.Context(),
			dbgen.CreateCompetitionEntryParams{EventID: f.id, TeamID: pair.team},
		); err != nil {
			t.Fatal(err)
		}
		f.exec(
			"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
			f.id,
			pair.team,
			pair.person,
		)
		if err := f.q.SetCompetitionState(
			t.Context(),
			dbgen.SetCompetitionStateParams{
				EventID:  f.id,
				PersonID: pair.person,
				State:    "confirmed",
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	f.exec(
		"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees) VALUES($1,true,true,2,2,2) ON CONFLICT(event_id) DO UPDATE SET total_teams=2,frozen_teams=2,assigned_attendees=2",
		f.id,
	)
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) saveConfig() {
	data, _ := json.Marshal(f.c)
	if _, err := f.q.SaveEventConfiguration(
		f.t.Context(),
		dbgen.SaveEventConfigurationParams{
			ID:            f.id,
			Name:          "Synthetic submissions",
			Slug:          uuid.NewString(),
			Configuration: data,
		},
	); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) saveDefs() {
	data, _ := json.Marshal(f.defs)
	if err := f.q.SaveEventDefinitions(
		f.t.Context(),
		dbgen.SaveEventDefinitionsParams{EventID: f.id, Definitions: data},
	); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) evidence(label string) []submissions.Evidence {
	return []submissions.Evidence{
		{
			DeliverableID: f.repo,
			Kind:          "repository",
			Label:         label,
			URL:           "https://example.test/team/" + label,
		},
	}
}

func (f *fixture) read(actor, team uuid.UUID) submissions.Page {
	f.t.Helper()
	v, err := f.s.Read(f.t.Context(), actor, f.id, team)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *fixture) submit(
	actor uuid.UUID,
	expected int64,
	items []submissions.Evidence,
	reason string,
) submissions.Page {
	f.t.Helper()
	v, err := f.s.Submit(
		f.t.Context(),
		actor,
		f.id,
		f.team,
		expected,
		submissions.Command{Evidence: items, Reason: reason},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *fixture) grant(person uuid.UUID, role string, target uuid.UUID) {
	f.t.Helper()
	if err := f.q.SetMembership(
		f.t.Context(),
		dbgen.SetMembershipParams{EventID: f.id, PersonID: person, Role: role, Active: true},
	); err != nil {
		f.t.Fatal(err)
	}
	f.exec(
		"INSERT INTO resource_assignments(event_id,resource_id,person_id) VALUES($1,$2,$3)",
		f.id,
		target,
		person,
	)
}

func TestSubmissionBoundariesHistoryAndAuthorization(t *testing.T) {
	pool := testdb.New(t, true)
	t.Run("private work and pre-live rejection", func(t *testing.T) {
		f := newFixture(t, pool)
		ctx := t.Context()
		f.now = f.c.LiveStart.Add(-time.Nanosecond)
		draft, err := f.s.SaveDraft(ctx, f.member.ID, f.id, f.team, 0, f.evidence("draft"))
		if err != nil || draft.Revision != 1 {
			t.Fatalf("draft: %+v %v", draft, err)
		}
		for _, actor := range []uuid.UUID{f.owner.ID, f.other.ID} {
			if _, err = f.s.Draft(ctx, actor, f.id, f.team); !errors.Is(err, access.ErrForbidden) {
				t.Fatalf("private draft exposed: %v", err)
			}
		}
		for _, actor := range []uuid.UUID{f.owner.ID, f.member.ID} {
			if _, err = f.s.Submit(
				ctx,
				actor,
				f.id,
				f.team,
				0,
				submissions.Command{
					Evidence: f.evidence("early"),
					Reason:   "Cannot bypass live start",
				},
			); !errors.Is(
				err,
				event.ErrBlocked,
			) {
				t.Fatalf("early submission accepted: %v", err)
			}
		}
		if _, err = f.s.Review(
			ctx,
			f.owner.ID,
			f.id,
			f.team,
			0,
			1,
			"accepted",
			"early",
		); !errors.Is(
			err,
			event.ErrBlocked,
		) {
			t.Fatalf("early review accepted: %v", err)
		}
		if f.read(f.owner.ID, f.team).Final != nil {
			t.Fatal("private draft treated as final")
		}
		admin := testdb.Person(t, pool)
		f.exec("UPDATE people SET platform_admin=true WHERE id=$1", admin.ID)
		if _, err = f.s.Read(ctx, admin.ID, f.id, f.team); !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("admin gained submission access: %v", err)
		}
		f.exec("UPDATE events SET published_at=NULL WHERE id=$1", f.id)
		if _, err = f.s.SaveDraft(
			ctx,
			f.member.ID,
			f.id,
			f.team,
			1,
			f.evidence("hidden"),
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatalf("unpublished draft accepted: %v", err)
		}
	})
	t.Run("required kinds immutable revisions and review history", func(t *testing.T) {
		f := newFixture(t, pool)
		ctx := t.Context()
		if _, err := f.s.Submit(
			ctx,
			f.member.ID,
			f.id,
			f.team,
			0,
			submissions.Command{Evidence: []submissions.Evidence{}},
		); !errors.Is(
			err,
			event.ErrBlocked,
		) {
			t.Fatalf("missing required accepted: %v", err)
		}
		bad := f.evidence("wrong")
		bad[0].Kind = "url"
		if _, err := f.s.Submit(
			ctx,
			f.member.ID,
			f.id,
			f.team,
			0,
			submissions.Command{Evidence: bad},
		); !errors.Is(
			err,
			event.ErrBlocked,
		) {
			t.Fatalf("wrong evidence kind accepted: %v", err)
		}
		bad = f.evidence("bad")
		bad[0].URL = "https://secret:password@example.test/repo"
		if _, err := f.s.Submit(
			ctx,
			f.member.ID,
			f.id,
			f.team,
			0,
			submissions.Command{Evidence: bad},
		); !errors.Is(
			err,
			event.ErrBlocked,
		) {
			t.Fatalf("embedded credentials accepted: %v", err)
		}
		first := f.submit(f.member.ID, 0, f.evidence("first"), "")
		reviewed, err := f.s.Review(
			ctx,
			f.owner.ID,
			f.id,
			f.team,
			first.Revision,
			first.Final.Revision,
			"accepted",
			"Required evidence checked",
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.Submit(
			ctx,
			f.member.ID,
			f.id,
			f.team,
			first.Revision,
			submissions.Command{Evidence: f.evidence("replacement")},
		); !errors.Is(
			err,
			access.ErrConflict,
		) {
			t.Fatalf("stale replacement accepted: %v", err)
		}
		second := f.submit(f.member.ID, reviewed.Revision, f.evidence("replacement"), "")
		if second.Final.ReviewStatus != "pending" || second.EntryID != first.EntryID {
			t.Fatalf("replacement state: %+v", second)
		}
		if _, err = f.s.Review(
			ctx,
			f.owner.ID,
			f.id,
			f.team,
			second.Revision,
			first.Final.Revision,
			"accepted",
			"Wrong revision",
		); !errors.Is(
			err,
			access.ErrConflict,
		) {
			t.Fatalf("superseded revision reviewed: %v", err)
		}
		history, err := f.s.History(ctx, f.member.ID, f.id, f.team, 0, 2)
		if err != nil || len(history.Items) != 2 || history.NextCursor == nil {
			t.Fatalf("history page: %+v %v", history, err)
		}
		tail, err := f.s.History(ctx, f.member.ID, f.id, f.team, *history.NextCursor, 2)
		if err != nil || len(tail.Items) != 1 ||
			tail.Items[0].Submission.Evidence[0].Label != "first" ||
			tail.Items[0].Submission.ReviewStatus != "accepted" {
			t.Fatalf("immutable earlier revision/review: %+v %v", tail, err)
		}
		for _, table := range []string{"submission_revisions", "submission_changes"} {
			if _, err = f.pool.Exec(
				ctx,
				"DELETE FROM "+table+" WHERE event_id=$1",
				f.id,
			); err == nil {
				t.Fatal("immutable history deleted")
			}
		}
		if err = f.q.CreateCompetitionEntry(
			ctx,
			dbgen.CreateCompetitionEntryParams{EventID: f.id, TeamID: f.team},
		); err != nil {
			t.Fatal(err)
		}
		var count int
		if err = f.pool.QueryRow(ctx, "SELECT count(*) FROM competition_entries WHERE event_id=$1 AND team_id=$2", f.id, f.team).
			Scan(&count); err != nil ||
			count != 1 {
			t.Fatalf("entry uniqueness: %d %v", count, err)
		}
	})
	t.Run(
		"cutoff settles without stage advancement and reasoned amendments preserve history",
		func(t *testing.T) {
			f := newFixture(t, pool)
			ctx := t.Context()
			f.now = f.c.SubmissionCutoff.Add(-time.Microsecond)
			first := f.submit(f.member.ID, 0, f.evidence("on-time"), "")
			f.now = f.c.SubmissionCutoff
			if _, err := f.s.Submit(
				ctx,
				f.member.ID,
				f.id,
				f.team,
				first.Revision,
				submissions.Command{Evidence: f.evidence("too-late")},
			); !errors.Is(
				err,
				event.ErrBlocked,
			) {
				t.Fatalf("ordinary cutoff write accepted: %v", err)
			}
			frozen := f.read(f.member.ID, f.team)
			if frozen.Outcome != "frozen" || frozen.FrozenAt == nil ||
				!frozen.FrozenAt.Equal(f.c.SubmissionCutoff) {
				t.Fatalf("clock freeze: %+v", frozen)
			}
			if missing := f.read(
				f.owner.ID,
				f.otherTeam,
			); missing.Outcome != "not_submitted" ||
				missing.Final != nil {
				t.Fatalf("missing outcome: %+v", missing)
			}
			if _, err := f.s.Submit(
				ctx,
				f.owner.ID,
				f.id,
				f.team,
				frozen.Revision,
				submissions.Command{Evidence: f.evidence("amended")},
			); !errors.Is(
				err,
				event.ErrBlocked,
			) {
				t.Fatalf("unreasoned amendment accepted: %v", err)
			}
			amended := f.submit(
				f.owner.ID,
				frozen.Revision,
				f.evidence("amended"),
				"Correct documented upload error",
			)
			if !amended.Final.Late || amended.Outcome != "frozen" ||
				amended.Final.Revision == first.Final.Revision {
				t.Fatalf("late amendment: %+v", amended)
			}
			f.now = f.now.Add(time.Hour)
			if err := f.s.Maintain(ctx); err != nil {
				t.Fatal(err)
			}
			again := f.read(f.owner.ID, f.team)
			if again.Revision != amended.Revision {
				t.Fatal("repeated settlement changed final")
			}
			f.exec("UPDATE events SET stage='live' WHERE id=$1", f.id)
			row, err := f.q.Event(ctx, f.id)
			if err != nil {
				t.Fatal(err)
			}
			events := event.Service{
				Pool:              pool,
				Clock:             func() time.Time { return f.now },
				SettleSubmissions: f.s.Settle,
			}
			if _, err = events.Transition(
				ctx,
				f.owner.ID,
				f.id,
				row.Revision,
				"assessment",
				"",
				false,
			); err != nil {
				t.Fatalf("settled gate: %v", err)
			}
		},
	)
	t.Run("transition settles missing outcomes even without worker or reads", func(t *testing.T) {
		f := newFixture(t, pool)
		ctx := t.Context()
		f.now = f.c.SubmissionCutoff
		f.exec("UPDATE events SET stage='live' WHERE id=$1", f.id)
		row, err := f.q.Event(ctx, f.id)
		if err != nil {
			t.Fatal(err)
		}
		service := event.Service{
			Pool:              pool,
			Clock:             func() time.Time { return f.now },
			SettleSubmissions: f.s.Settle,
		}
		if _, err = service.Transition(
			ctx,
			f.owner.ID,
			f.id,
			row.Revision,
			"assessment",
			"",
			false,
		); err != nil {
			t.Fatal(err)
		}
		if f.read(f.owner.ID, f.team).Outcome != "not_submitted" {
			t.Fatal("gate passed without explicit outcome")
		}
	})
	t.Run("earlier deadlines lock only changed evidence", func(t *testing.T) {
		f := newFixture(t, pool)
		ctx := t.Context()
		agenda := uuid.New()
		f.c.Schedule = []event.AgendaItem{
			{
				ID:       agenda,
				Title:    "Repository checkpoint",
				Anchor:   "live_start",
				Minutes:  60,
				Audience: []string{"attendee"},
			},
		}
		f.defs.Deliverables[0].ScheduleID = &agenda
		f.saveConfig()
		f.saveDefs()
		first := f.submit(f.member.ID, 0, f.evidence("checkpoint"), "")
		f.now = f.c.LiveStart.Add(2 * time.Hour)
		if _, err := f.s.Submit(
			ctx,
			f.member.ID,
			f.id,
			f.team,
			first.Revision,
			submissions.Command{Evidence: f.evidence("changed")},
		); !errors.Is(
			err,
			event.ErrBlocked,
		) {
			t.Fatalf("early deadline bypass: %v", err)
		}
		items := append(
			f.evidence("checkpoint"),
			submissions.Evidence{
				DeliverableID: f.attachment,
				Kind:          "url",
				Label:         "Demo",
				URL:           "https://example.test/demo",
			},
		)
		second := f.submit(f.member.ID, first.Revision, items, "")
		if second.Final.Late {
			t.Fatal("unchanged earlier evidence marked late")
		}
		amended := f.submit(
			f.owner.ID,
			second.Revision,
			f.evidence("correction"),
			"Correct recorded repository reference",
		)
		if !amended.Final.Late {
			t.Fatal("early deadline override not marked late")
		}
	})
	t.Run(
		"postponement cannot reopen a final that froze while the worker was delayed",
		func(t *testing.T) {
			f := newFixture(t, pool)
			ctx := t.Context()
			first := f.submit(f.member.ID, 0, f.evidence("original"), "")
			originalCutoff := f.c.SubmissionCutoff
			f.now = originalCutoff.Add(time.Minute)
			f.c.LiveStart = f.c.LiveStart.Add(7 * 24 * time.Hour)
			f.c.LiveEnd = f.c.LiveEnd.Add(7 * 24 * time.Hour)
			f.c.SubmissionCutoff = f.c.SubmissionCutoff.Add(7 * 24 * time.Hour)
			row, err := f.q.Event(ctx, f.id)
			if err != nil {
				t.Fatal(err)
			}
			events := event.Service{
				Pool:              pool,
				Clock:             func() time.Time { return f.now },
				SettleSubmissions: f.s.Settle,
			}
			if _, err = events.Update(
				ctx,
				f.owner.ID,
				f.id,
				row.Revision,
				event.Input{
					Name:                row.Name,
					Slug:                row.Slug,
					Configuration:       f.c,
					ApprovePostponement: true,
					Reason:              "Exceptional postponement communicated to every team",
				},
			); err != nil {
				t.Fatal(err)
			}
			f.now = f.c.LiveStart
			frozen := f.read(f.member.ID, f.team)
			if frozen.Outcome != "frozen" || frozen.CanSubmit ||
				frozen.Final.Revision != first.Final.Revision ||
				!frozen.FrozenAt.Equal(originalCutoff) {
				t.Fatalf("postponement reopened final: %+v", frozen)
			}
			if _, err = f.s.Submit(
				ctx,
				f.member.ID,
				f.id,
				f.team,
				frozen.Revision,
				submissions.Command{Evidence: f.evidence("unapproved")},
			); !errors.Is(
				err,
				event.ErrBlocked,
			) {
				t.Fatalf("frozen entry changed: %v", err)
			}
			amended := f.submit(
				f.owner.ID,
				frozen.Revision,
				f.evidence("approved"),
				"Exceptional correction to frozen evidence",
			)
			if amended.Outcome != "frozen" || !amended.Final.Late ||
				!amended.FrozenAt.Equal(originalCutoff) {
				t.Fatalf("amendment reopened final: %+v", amended)
			}
		},
	)
	t.Run(
		"published requirements stay fixed and linked deadline edits need a reason",
		func(t *testing.T) {
			f := newFixture(t, pool)
			ctx := t.Context()
			agenda := uuid.New()
			f.c.Schedule = []event.AgendaItem{
				{
					ID:       agenda,
					Title:    "Repository deadline",
					Anchor:   "live_start",
					Minutes:  60,
					Audience: []string{"attendee"},
				},
			}
			f.defs.Deliverables[0].ScheduleID = &agenda
			f.saveConfig()
			f.saveDefs()
			row, err := f.q.Event(ctx, f.id)
			if err != nil {
				t.Fatal(err)
			}
			changed := f.defs
			changed.Deliverables = append([]templates.Deliverable{}, f.defs.Deliverables...)
			changed.Deliverables[0].Required = false
			if _, err = (templates.Service{Pool: pool}).SaveDefinitions(
				ctx,
				f.owner.ID,
				f.id,
				row.Revision,
				changed,
			); !errors.Is(
				err,
				event.ErrBlocked,
			) {
				t.Fatalf("published requirement altered: %v", err)
			}
			events := event.Service{
				Pool:              pool,
				Clock:             func() time.Time { return f.now },
				SettleSubmissions: f.s.Settle,
			}
			f.c.Schedule[0].Minutes = 180
			input := event.Input{Name: row.Name, Slug: row.Slug, Configuration: f.c}
			if _, err = events.Update(
				ctx,
				f.owner.ID,
				f.id,
				row.Revision,
				input,
			); !errors.Is(
				err,
				event.ErrBlocked,
			) {
				t.Fatalf("deadline edited without reason: %v", err)
			}
			input.Reason = "Extend the checkpoint for all teams after venue outage"
			if _, err = events.Update(ctx, f.owner.ID, f.id, row.Revision, input); err != nil {
				t.Fatal(err)
			}
			var n int
			if err = f.pool.QueryRow(ctx, "SELECT count(*) FROM event_decisions WHERE event_id=$1 AND kind='deliverable_deadline_change' AND reason=$2", f.id, input.Reason).
				Scan(&n); err != nil ||
				n != 1 {
				t.Fatalf("deadline reason absent: %d %v", n, err)
			}
		},
	)
	t.Run("inactive people and read-only events cannot change evidence", func(t *testing.T) {
		f := newFixture(t, pool)
		ctx := t.Context()
		first := f.submit(f.member.ID, 0, f.evidence("saved"), "")
		f.exec("UPDATE events SET writes_allowed=false WHERE id=$1", f.id)
		if _, err := f.s.Submit(
			ctx,
			f.member.ID,
			f.id,
			f.team,
			first.Revision,
			submissions.Command{Evidence: f.evidence("forbidden")},
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatalf("read-only write accepted: %v", err)
		}
		if _, err := f.s.Review(
			ctx,
			f.owner.ID,
			f.id,
			f.team,
			first.Revision,
			first.Final.Revision,
			"accepted",
			"No authority",
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatalf("read-only review accepted: %v", err)
		}
		f.exec("UPDATE people SET active=false WHERE id=$1", f.member.ID)
		if _, err := f.s.Read(
			ctx,
			f.member.ID,
			f.id,
			f.team,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatalf("inactive person read evidence: %v", err)
		}
	})
	t.Run("competing saves and duplicate cutoff workers serialize", func(t *testing.T) {
		f := newFixture(t, pool)
		ctx := t.Context()
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for i := range 2 {
			wg.Go(func() {
				_, err := f.s.Submit(
					ctx,
					f.member.ID,
					f.id,
					f.team,
					0,
					submissions.Command{Evidence: f.evidence(fmt.Sprint(i))},
				)
				results <- err
			})
		}
		wg.Wait()
		close(results)
		ok, conflicts := 0, 0
		for err := range results {
			if err == nil {
				ok++
			} else if errors.Is(err, access.ErrConflict) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
		if ok != 1 || conflicts != 1 {
			t.Fatalf("save race: %d %d", ok, conflicts)
		}
		f.now = f.c.SubmissionCutoff
		for range 2 {
			wg.Go(func() {
				if err := f.s.Maintain(ctx); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		var count int
		if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM submission_changes WHERE event_id=$1 AND team_id=$2 AND action='frozen'", f.id, f.team).
			Scan(&count); err != nil ||
			count != 1 {
			t.Fatalf("freeze dedup: %d %v", count, err)
		}
	})
	t.Run(
		"withdrawn and disbanded teams are unscored and membership revocation is immediate",
		func(t *testing.T) {
			f := newFixture(t, pool)
			ctx := t.Context()
			first := f.submit(f.member.ID, 0, f.evidence("entry"), "")
			withdrawn, err := f.s.Outcome(
				ctx,
				f.member.ID,
				f.id,
				f.team,
				first.Revision,
				"withdrawn",
				"Team chooses to withdraw",
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.Submit(
				ctx,
				f.member.ID,
				f.id,
				f.team,
				withdrawn.Revision,
				submissions.Command{Evidence: f.evidence("revived")},
			); !errors.Is(
				err,
				event.ErrBlocked,
			) {
				t.Fatalf("withdrawn entry revived: %v", err)
			}
			f.exec(
				"UPDATE competition_entries SET active=false WHERE event_id=$1 AND team_id=$2",
				f.id,
				f.otherTeam,
			)
			f.exec(
				"UPDATE hack_teams SET state='disbanded' WHERE event_id=$1 AND id=$2",
				f.id,
				f.otherTeam,
			)
			f.now = f.c.SubmissionCutoff
			other := f.read(f.owner.ID, f.otherTeam)
			if other.Outcome != "withdrawn" {
				t.Fatalf("disband outcome: %+v", other)
			}
			if _, err = f.s.Review(
				ctx,
				f.owner.ID,
				f.id,
				f.team,
				f.read(f.owner.ID, f.team).Revision,
				first.Final.Revision,
				"accepted",
				"Invalid",
			); !errors.Is(
				err,
				access.ErrForbidden,
			) {
				t.Fatalf("withdrawn reviewed: %v", err)
			}
			f.exec(
				"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
				f.id,
				f.member.ID,
			)
			if _, err = f.s.Read(
				ctx,
				f.member.ID,
				f.id,
				f.team,
			); !errors.Is(
				err,
				access.ErrForbidden,
			) {
				t.Fatalf("former member retained evidence: %v", err)
			}
		},
	)
}

func TestTeamFilesAndReviewerAudiences(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	ctx := t.Context()
	storage, err := files.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	fileService := files.Service{
		Pool:       pool,
		Storage:    storage,
		Policy:     access.EventPolicy{Queries: f.q, Clock: func() time.Time { return f.now }},
		MaxBytes:   1000,
		EventQuota: 10000,
	}
	f.now = f.c.LiveStart.Add(-time.Hour)
	row, err := f.q.Event(ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	input := files.Upload{
		Actor:     f.member.ID,
		Event:     f.id,
		TeamID:    &f.team,
		Key:       uuid.New(),
		Revision:  row.Revision,
		Filename:  "notes.txt",
		MediaType: "text/plain",
		Content:   []byte("synthetic private evidence"),
	}
	file, err := fileService.Upload(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := fileService.Upload(ctx, input)
	if err != nil || replay.ID != file.ID {
		t.Fatalf("upload retry: %v", err)
	}
	f.grant(f.mentor.ID, "mentor", f.team)
	entry, err := f.q.CompetitionEntry(
		ctx,
		dbgen.CompetitionEntryParams{EventID: f.id, TeamID: f.team},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.grant(f.judge.ID, "judge", entry.ID)
	for _, actor := range []uuid.UUID{f.owner.ID, f.other.ID, f.mentor.ID, f.judge.ID} {
		if _, _, err = fileService.Download(
			ctx,
			actor,
			f.id,
			file.ID,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatalf("pre-live file leaked: %v", err)
		}
	}
	if _, _, err = fileService.Download(ctx, f.member.ID, f.id, file.ID); err != nil {
		t.Fatal(err)
	}
	f.now = f.c.LiveStart
	if _, _, err = fileService.Download(
		ctx,
		f.owner.ID,
		f.id,
		file.ID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("unsubmitted private file leaked: %v", err)
	}
	items := append(
		f.evidence("formal"),
		submissions.Evidence{
			DeliverableID: f.attachment,
			Kind:          "file",
			Label:         "Technical notes",
			FileID:        &file.ID,
		},
	)
	first := f.submit(f.member.ID, 0, items, "")
	if _, _, err = fileService.Download(ctx, f.mentor.ID, f.id, file.ID); err != nil {
		t.Fatalf("assigned mentor denied formal evidence: %v", err)
	}
	if _, err = f.s.Read(ctx, f.judge.ID, f.id, f.team); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("judge before cutoff: %v", err)
	}
	if _, err = f.s.Submit(
		ctx,
		f.other.ID,
		f.id,
		f.otherTeam,
		0,
		submissions.Command{Evidence: items},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("cross-team attachment accepted: %v", err)
	}
	input.Key = uuid.New()
	input.Filename = "private-notes.txt"
	input.Content = []byte("restricted formal evidence")
	secret, err := fileService.Upload(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	items = append(
		items,
		submissions.Evidence{
			DeliverableID: f.secret,
			Kind:          "file",
			Label:         "Private appendix",
			FileID:        &secret.ID,
		},
	)
	second := f.submit(f.member.ID, first.Revision, items, "")
	mentor := f.read(f.mentor.ID, f.team)
	if len(mentor.Final.Evidence) != 2 {
		t.Fatal("review audience exposed restricted evidence")
	}
	if _, _, err = fileService.Download(
		ctx,
		f.mentor.ID,
		f.id,
		secret.ID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("restricted direct file leaked: %v", err)
	}
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		f.id,
		f.mentor.ID,
	)
	if _, _, err = fileService.Download(
		ctx,
		f.mentor.ID,
		f.id,
		file.ID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("revoked mentor retained file: %v", err)
	}
	f.now = f.c.SubmissionCutoff
	frozen := f.read(f.owner.ID, f.team)
	if _, _, err = fileService.Download(ctx, f.judge.ID, f.id, file.ID); err != nil {
		t.Fatalf("judge cannot read frozen file: %v", err)
	}
	changed := f.submit(
		f.owner.ID,
		frozen.Revision,
		f.evidence("amended-without-file"),
		"Remove erroneous attachment",
	)
	if !changed.Final.Late || changed.Final.Revision == second.Final.Revision {
		t.Fatal("late immutable replacement missing")
	}
	if _, _, err = fileService.Download(
		ctx,
		f.judge.ID,
		f.id,
		file.ID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("judge read superseded final attachment: %v", err)
	}
	input.Actor = f.owner.ID
	input.Key = uuid.New()
	input.Filename = "late.txt"
	if _, err = fileService.Upload(ctx, input); !errors.Is(err, access.ErrInvalid) {
		t.Fatalf("unreasoned late file accepted: %v", err)
	}
	input.Reason = "Replace corrupt upload"
	if _, err = fileService.Upload(ctx, input); err != nil {
		t.Fatal(err)
	}
	input.TeamID = &f.otherTeam
	if _, err = fileService.Upload(ctx, input); !errors.Is(err, files.ErrIdempotency) {
		t.Fatalf("upload key reused across scopes: %v", err)
	}
}

func TestSubmissionWorkspaceDraftAuthorityAndCurrentOrganisers(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	f.now = f.c.LiveStart.Add(-time.Hour)
	member := f.read(f.member.ID, f.team)
	owner := f.read(f.owner.ID, f.team)
	if !member.CanReadDraft || !member.CanDraft || member.CanSubmit || owner.CanReadDraft || owner.CanDraft || owner.CanSubmit {
		t.Fatalf("pre-live workspace authority: member=%+v owner=%+v", member, owner)
	}
	if len(member.Organisers) != 1 || member.Organisers[0].ID != f.owner.ID || member.Organisers[0].Name != f.owner.DisplayName {
		t.Fatalf("organiser contact: %+v", member.Organisers)
	}
	second := testdb.Person(t, pool)
	f.exec("INSERT INTO event_memberships(event_id,person_id,role,active) VALUES($1,$2,'organiser',true)", f.id, second.ID)
	if got := f.read(f.member.ID, f.team); len(got.Organisers) != 2 {
		t.Fatal("new active organiser unavailable")
	}
	f.exec("UPDATE people SET active=false WHERE id=$1", second.ID)
	if got := f.read(f.member.ID, f.team); len(got.Organisers) != 1 {
		t.Fatal("disabled organiser disclosed as contact")
	}
	f.exec("UPDATE people SET active=true WHERE id=$1", second.ID)
	f.exec("UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'", f.id, second.ID)
	if got := f.read(f.member.ID, f.team); len(got.Organisers) != 1 {
		t.Fatal("revoked organiser disclosed as contact")
	}
	f.now = f.c.SubmissionCutoff
	member = f.read(f.member.ID, f.team)
	if !member.CanReadDraft || member.CanDraft || member.CanSubmit || member.Outcome != "not_submitted" {
		t.Fatalf("cutoff workspace authority: %+v", member)
	}
	f.exec("UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2", f.id, f.member.ID)
	if _, err := f.s.Read(t.Context(), f.member.ID, f.id, f.team); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("revoked workspace access: %v", err)
	}
}
