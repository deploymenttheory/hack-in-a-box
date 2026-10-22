package content_test

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	t                           *testing.T
	pool                        *pgxpool.Pool
	q                           *dbgen.Queries
	s                           content.Service
	fs                          files.Service
	owner, member, other, early dbgen.Person
	id, team                    uuid.UUID
	c                           event.Configuration
	now, due                    time.Time
}

func newFixture(t *testing.T, pool *pgxpool.Pool) *fixture {
	t.Helper()
	f := &fixture{
		t:      t,
		pool:   pool,
		q:      dbgen.New(pool),
		owner:  testdb.Person(t, pool),
		member: testdb.Person(t, pool),
		other:  testdb.Person(t, pool),
		early:  testdb.Person(t, pool),
		team:   uuid.New(),
	}
	f.id = testdb.Event(t, pool, f.owner.ID)
	f.c = event.DefaultConfiguration()
	f.c.Timezone = "Europe/London"
	f.c.MinimumAttendance = 1
	zone, _ := time.LoadLocation(f.c.Timezone)
	f.c.LiveStart = time.Date(2027, 3, 29, 9, 0, 0, 0, zone)
	f.c.LiveEnd = f.c.LiveStart.Add(8 * time.Hour)
	f.c.SubmissionCutoff = f.c.LiveEnd.Add(-time.Hour)
	f.due, _ = schedule.ChallengePublication(f.c.LiveStart, f.c.Timezone, f.c.DSTChoice)
	f.c.TeamFreeze = f.due.Add(-time.Hour)
	f.c.AdmissionCutoff = f.c.TeamFreeze.Add(-time.Hour)
	f.now = f.due.Add(-time.Minute)
	f.s = content.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	data, _ := json.Marshal(f.c)
	f.exec(
		"UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=$3,slug=$4 WHERE id=$1",
		f.id,
		data,
		f.c.TeamFreeze,
		uuid.NewString(),
	)
	f.exec(
		"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees) VALUES($1,true,true,1,1,1)",
		f.id,
	)
	f.exec(
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		f.id,
		f.member.ID,
	)
	f.exec(
		"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Test team','published',true)",
		f.id,
		f.team,
	)
	f.exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		f.id,
		f.team,
		f.member.ID,
	)
	store, err := files.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	f.s.Storage = store
	f.fs = files.Service{
		Pool:       pool,
		Storage:    store,
		Policy:     access.EventPolicy{Queries: f.q, Clock: func() time.Time { return f.now }},
		MaxBytes:   1000,
		EventQuota: 100000,
	}
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) row() dbgen.Event {
	f.t.Helper()
	v, err := f.q.Event(f.t.Context(), f.id)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *fixture) save(old content.Page, in content.Input) content.Page {
	f.t.Helper()
	if old.ID == uuid.Nil {
		old.ID = uuid.New()
	}
	v, err := f.s.Save(f.t.Context(), f.owner.ID, f.id, old.ID, old.Revision, in)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *fixture) act(p content.Page, a string) content.Page {
	f.t.Helper()
	v, err := f.s.Action(f.t.Context(), f.owner.ID, f.id, p.ID, p.Revision, a, "Synthetic decision")
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *fixture) approve(
	p content.Page,
) content.Page {
	return f.act(f.act(p, "review"), "approve")
}

func (f *fixture) preview(p content.Page) content.Page {
	f.t.Helper()
	v, err := f.s.Read(f.t.Context(), f.owner.ID, f.id, p.ID, true, 0)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *fixture) dispatch(p content.Page) {
	f.t.Helper()
	if err := f.s.Dispatch(f.t.Context(), f.id, p.ID, p.Revision); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) upload() uuid.UUID {
	f.t.Helper()
	file, err := f.fs.Upload(
		f.t.Context(),
		files.Upload{
			Actor:     f.owner.ID,
			Event:     f.id,
			Revision:  f.row().Revision,
			Key:       uuid.New(),
			Filename:  "notes.txt",
			MediaType: "text/plain",
			Content:   []byte("Synthetic protected attachment"),
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return file.ID
}

func input(kind string) content.Input {
	return content.Input{
		Kind: kind,
		Document: content.Document{
			Title:    "Synthetic challenge",
			Markdown: "# Challenge\nBuild something useful.",
			Audience: access.ContentAudience{
				Confirmed: true,
				Roles:     []string{},
				Teams:     []uuid.UUID{},
			},
			Attachments: []uuid.UUID{},
		},
	}
}

func expectBlocked(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, event.ErrBlocked) {
		t.Fatalf("expected domain block, got %v", err)
	}
}

func TestContentWorkflowAndEmbargo(t *testing.T) {
	pool := testdb.New(t, true)
	t.Run("metadata attachments and lasting privileged exclusion", func(t *testing.T) {
		f := newFixture(t, pool)
		ctx := t.Context()
		in := input("challenge")
		file := f.upload()
		in.Document.Attachments = []uuid.UUID{file}
		p := f.save(content.Page{}, in)
		if f.c.LiveStart.Sub(*p.Document.ScheduledAt) != 167*time.Hour ||
			p.Document.ScheduledAt.In(time.UTC).Hour() != 9 {
			t.Fatal("publication lost calendar-day DST semantics")
		}
		for _, actor := range []uuid.UUID{f.member.ID, f.other.ID} {
			if _, err := f.s.Read(
				ctx,
				actor,
				f.id,
				p.ID,
				false,
				0,
			); !errors.Is(
				err,
				access.ErrNotFound,
			) {
				t.Fatalf("title leaked: %v", err)
			}
			if _, _, err := f.fs.Download(
				ctx,
				actor,
				f.id,
				file,
			); !errors.Is(
				err,
				access.ErrForbidden,
			) {
				t.Fatalf("file leaked: %v", err)
			}
		}
		list, err := f.s.List(ctx, f.member.ID, f.id, uuid.Nil, 20, false)
		if err != nil || len(list.Items) != 0 || list.NextCursor != nil {
			t.Fatalf("hidden list: %+v %v", list, err)
		}
		if _, err = f.s.Read(
			ctx,
			f.member.ID,
			f.id,
			p.ID,
			true,
			0,
		); !errors.Is(
			err,
			access.ErrNotFound,
		) {
			t.Fatalf("preview bypass: %v", err)
		}
		if _, err = f.q.BootstrapAdmin(ctx, f.other.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.Read(
			ctx,
			f.other.ID,
			f.id,
			p.ID,
			true,
			0,
		); !errors.Is(
			err,
			access.ErrNotFound,
		) {
			t.Fatal("platform admin preview bypass")
		}
		change := permissions.Change{
			Actor:                             f.owner.ID,
			Event:                             f.id,
			Person:                            f.member.ID,
			Role:                              "early_brief_access",
			Active:                            true,
			AcknowledgeCompetitionRestriction: true,
			Revision:                          f.row().Revision,
		}
		if _, err = (permissions.Service{Pool: pool}).Change(
			ctx,
			change,
		); !errors.Is(
			err,
			permissions.ErrWithdrawal,
		) {
			t.Fatalf("active competitor grant: %v", err)
		}
		f.exec(
			"UPDATE competition_claims SET state='waitlisted' WHERE event_id=$1 AND person_id=$2",
			f.id,
			f.member.ID,
		)
		if _, err = (permissions.Service{Pool: pool}).Change(
			ctx,
			change,
		); !errors.Is(
			err,
			permissions.ErrWithdrawal,
		) {
			t.Fatalf("waitlisted grant: %v", err)
		}
		change.Person = f.early.ID
		if _, err = (permissions.Service{Pool: pool}).Change(ctx, change); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.Read(ctx, f.early.ID, f.id, p.ID, true, 0); err != nil {
			t.Fatal(err)
		}
		if _, _, err = f.fs.Download(ctx, f.early.ID, f.id, file); err != nil {
			t.Fatal(err)
		}
		change.Active = false
		change.Revision = f.row().Revision
		if _, err = (permissions.Service{Pool: pool}).Change(ctx, change); err != nil {
			t.Fatal(err)
		}
		if _, _, err = f.fs.Download(
			ctx,
			f.early.ID,
			f.id,
			file,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal("revoked grant retained copied download")
		}
		excluded, err := f.q.CompetitionExcluded(
			ctx,
			dbgen.CompetitionExcludedParams{EventID: f.id, PersonID: f.early.ID},
		)
		if err != nil || !excluded {
			t.Fatalf("grant never opened or revoked erased exclusion: %v", err)
		}
		in.Kind = "page"
		if _, err = f.s.Save(
			ctx,
			f.owner.ID,
			f.id,
			p.ID,
			p.Revision,
			in,
		); !errors.Is(
			err,
			event.ErrBlocked,
		) {
			t.Fatal("challenge relabelled")
		}
		if _, err = f.s.Save(
			ctx,
			f.owner.ID,
			f.id,
			uuid.New(),
			0,
			in,
		); !errors.Is(
			err,
			event.ErrBlocked,
		) {
			t.Fatal("attachment alias bypass")
		}
	})
	t.Run(
		"immutable versions approval invalidation and released version remains visible",
		func(t *testing.T) {
			f := newFixture(t, pool)
			ctx := t.Context()
			in := input("page")
			due := f.now.Add(time.Hour)
			in.Document.ScheduledAt = &due
			p := f.approve(f.save(content.Page{}, in))
			stale := p
			in.Document.Markdown = "Replacement draft"
			in.Document.ScheduledAt = nil
			p = f.save(p, in)
			f.now = due
			f.dispatch(stale)
			if f.preview(p).State != "draft" {
				t.Fatal("stale job published replacement")
			}
			p = f.act(f.approve(p), "release")
			in.Document.Markdown = "Unapproved secret change"
			draft := f.save(p, in)
			got, err := f.s.Read(ctx, f.member.ID, f.id, p.ID, false, 0)
			if err != nil || got.Document.Markdown != "Replacement draft" || got.Version != 2 {
				t.Fatalf("draft leaked over released version: %+v %v", got, err)
			}
			if _, err = f.s.Save(
				ctx,
				f.owner.ID,
				f.id,
				p.ID,
				p.Revision,
				in,
			); !errors.Is(
				err,
				access.ErrConflict,
			) {
				t.Fatal("stale save accepted")
			}
			history, err := f.s.History(ctx, f.owner.ID, f.id, p.ID, 0, 2)
			if err != nil || len(history.Items) != 2 || history.NextCursor == nil {
				t.Fatalf("history pagination %+v %v", history, err)
			}
			old, err := f.s.Read(ctx, f.owner.ID, f.id, p.ID, true, 1)
			if err != nil || old.Document.Markdown != input("page").Document.Markdown {
				t.Fatalf("historical document lost: %v", err)
			}
			if _, err = f.s.History(
				ctx,
				f.member.ID,
				f.id,
				p.ID,
				0,
				20,
			); !errors.Is(
				err,
				access.ErrForbidden,
			) {
				t.Fatal("participant read private history")
			}
			if _, err = pool.Exec(
				ctx,
				"UPDATE content_versions SET document='{}' WHERE event_id=$1",
				f.id,
			); err == nil {
				t.Fatal("content history mutable")
			}
			if _, err = pool.Exec(
				ctx,
				"DELETE FROM content_changes WHERE event_id=$1",
				f.id,
			); err == nil {
				t.Fatal("release history mutable")
			}
			p = f.act(draft, "withdraw")
			if !strings.Contains(p.Message, "cannot be recalled") {
				t.Fatal("withdrawal copy limitation missing")
			}
			if _, err = f.s.Read(
				ctx,
				f.member.ID,
				f.id,
				p.ID,
				false,
				0,
			); !errors.Is(
				err,
				access.ErrNotFound,
			) {
				t.Fatal("withdrawn page visible")
			}
		},
	)
	t.Run("manual page workflow role and team audiences", func(t *testing.T) {
		f := newFixture(t, pool)
		ctx := t.Context()
		file := f.upload()
		in := input("page")
		in.Document.Audience = access.ContentAudience{Teams: []uuid.UUID{f.team}}
		in.Document.Attachments = []uuid.UUID{file}
		p := f.act(f.approve(f.save(content.Page{}, in)), "release")
		if _, _, err := f.fs.Download(ctx, f.member.ID, f.id, file); err != nil {
			t.Fatal(err)
		}
		if _, _, err := f.fs.Download(
			ctx,
			f.other.ID,
			f.id,
			file,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal("cross team attachment access")
		}
		f.exec(
			"DELETE FROM hack_team_members WHERE event_id=$1 AND person_id=$2",
			f.id,
			f.member.ID,
		)
		if _, _, err := f.fs.Download(
			ctx,
			f.member.ID,
			f.id,
			file,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal("removed membership retained access")
		}
		in.Document.Audience = access.ContentAudience{Roles: []string{"logistics"}}
		p = f.act(f.approve(f.save(p, in)), "release")
		f.exec(
			"INSERT INTO event_memberships(event_id,person_id,role,active) VALUES($1,$2,'logistics',true)",
			f.id,
			f.other.ID,
		)
		if _, _, err := f.fs.Download(ctx, f.other.ID, f.id, file); err != nil {
			t.Fatal(err)
		}
		f.exec(
			"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
			f.id,
			f.other.ID,
		)
		if _, _, err := f.fs.Download(
			ctx,
			f.other.ID,
			f.id,
			file,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal("revoked role retained access")
		}
		f.act(p, "withdraw")
		if _, _, err := f.fs.Download(
			ctx,
			f.owner.ID,
			f.id,
			file,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal("withdrawn attachment fallback bypass")
		}
	})
	t.Run("approval validation and cross event attachment", func(t *testing.T) {
		f := newFixture(t, pool)
		in := input("page")
		in.Document.Markdown = "{{unfinished}}"
		p := f.act(f.save(content.Page{}, in), "review")
		_, err := f.s.Action(t.Context(), f.owner.ID, f.id, p.ID, p.Revision, "approve", "")
		expectBlocked(t, err)
		in.Document.Markdown = "Ready"
		due := f.now.Add(-time.Second)
		in.Document.ScheduledAt = &due
		p = f.act(f.save(p, in), "review")
		_, err = f.s.Action(t.Context(), f.owner.ID, f.id, p.ID, p.Revision, "approve", "")
		expectBlocked(t, err)
		other := newFixture(t, pool)
		in.Document.Attachments = []uuid.UUID{other.upload()}
		_, err = f.s.Save(t.Context(), f.owner.ID, f.id, p.ID, p.Revision, in)
		expectBlocked(t, err)
		in = input("challenge")
		in.Document.Audience.Teams = []uuid.UUID{f.team}
		_, err = f.s.Save(t.Context(), f.owner.ID, f.id, uuid.New(), 0, in)
		expectBlocked(t, err)
		in = input("brief")
		p = f.approve(f.save(content.Page{}, in))
		_, err = f.s.Action(t.Context(), f.owner.ID, f.id, p.ID, p.Revision, "release", "")
		expectBlocked(t, err)
	})
}

func TestChallengePublicationGatesTimingAndRaces(t *testing.T) {
	pool := testdb.New(t, true)
	for _, tc := range []struct {
		name      string
		offset    time.Duration
		state     string
		breakGate string
	}{
		{"early", -time.Nanosecond, "approved", ""},
		{"exact", 0, "released", ""},
		{"last allowed instant", 60 * time.Second, "released", ""},
		{"missed", 60*time.Second + time.Nanosecond, "missed", ""},
		{"wrong stage", 0, "blocked", "UPDATE events SET stage='registration' WHERE id=$1"},
		{"attendance minimum", 0, "blocked", "UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1"},
		{"unfrozen team", 0, "blocked", "UPDATE event_gate_facts SET frozen_teams=0 WHERE event_id=$1"},
		{"unassigned attendee", 0, "blocked", "UPDATE event_gate_facts SET assigned_attendees=0 WHERE event_id=$1"},
		{"missing attendance facts", 0, "blocked", "UPDATE event_gate_facts SET attendance_ready=false WHERE event_id=$1"},
		{"cancelled event", 0, "blocked", "UPDATE events SET stage='cancelled',writes_allowed=false WHERE id=$1"},
		{"revoked approver", 0, "blocked", "UPDATE event_memberships SET active=false WHERE event_id=$1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, pool)
			p := f.approve(f.save(content.Page{}, input("challenge")))
			if tc.breakGate != "" {
				f.exec(tc.breakGate, f.id)
			}
			f.now = f.due.Add(tc.offset)
			f.dispatch(p)
			row, err := f.q.ContentItem(
				t.Context(),
				dbgen.ContentItemParams{EventID: f.id, ID: p.ID},
			)
			if err != nil || row.State != tc.state {
				t.Fatalf("state %s want %s: %v", row.State, tc.state, err)
			}
			published := f.row().PublishedAt
			if published.Valid != (tc.state == "released") {
				t.Fatalf("publication gate bypass: %+v", published)
			}
			if published.Valid && !published.Time.Equal(f.now) {
				t.Fatal("actual publication instant not recorded")
			}
			f.now = f.due.Add(2 * time.Hour)
			f.dispatch(p)
			if f.row().PublishedAt.Valid != published.Valid {
				t.Fatal("failed job caught up late")
			}
		})
	}
	t.Run("manual challenge bypass rejected", func(t *testing.T) {
		f := newFixture(t, pool)
		p := f.approve(f.save(content.Page{}, input("challenge")))
		_, err := f.s.Action(t.Context(), f.owner.ID, f.id, p.ID, p.Revision, "release", "")
		expectBlocked(t, err)
	})
	t.Run("cancellation and duplicate worker race", func(t *testing.T) {
		for range 8 {
			f := newFixture(t, pool)
			p := f.approve(f.save(content.Page{}, input("challenge")))
			f.now = f.due
			var wg sync.WaitGroup
			for range 3 {
				wg.Go(func() {
					if err := f.s.Dispatch(t.Context(), f.id, p.ID, p.Revision); err != nil {
						t.Error(err)
					}
				})
			}
			wg.Go(func() {
				_, err := f.s.Action(
					t.Context(),
					f.owner.ID,
					f.id,
					p.ID,
					p.Revision,
					"cancel",
					"Race cancellation",
				)
				if err != nil && !errors.Is(err, access.ErrConflict) {
					t.Error(err)
				}
			})
			wg.Wait()
			var count int
			if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM content_changes WHERE event_id=$1 AND action='released'", f.id).
				Scan(&count); err != nil {
				t.Fatal(err)
			}
			current := f.preview(p)
			if count > 1 || (current.State == "cancelled" && count != 0) ||
				(current.State == "released" && count != 1) {
				t.Fatalf("non atomic release: %s %d", current.State, count)
			}
			if f.row().PublishedAt.Valid != (count == 1) {
				t.Fatal("release and publication diverged")
			}
		}
	})
	t.Run("cancelled job stays cancelled", func(t *testing.T) {
		f := newFixture(t, pool)
		p := f.approve(f.save(content.Page{}, input("challenge")))
		f.act(p, "cancel")
		f.now = f.due
		f.dispatch(p)
		if f.row().PublishedAt.Valid {
			t.Fatal("cancelled job published")
		}
	})
}

func TestContentReschedulingAndNotices(t *testing.T) {
	pool := testdb.New(t, true)
	t.Run("reschedule invalidates approval and stale job", func(t *testing.T) {
		f := newFixture(t, pool)
		p := f.approve(f.save(content.Page{}, input("challenge")))
		original := f.due
		row := f.row()
		c := f.c
		c.LiveStart = c.LiveStart.AddDate(0, 0, 7)
		c.LiveEnd = c.LiveEnd.AddDate(0, 0, 7)
		c.SubmissionCutoff = c.SubmissionCutoff.AddDate(0, 0, 7)
		_, err := (event.Service{Pool: pool, Clock: func() time.Time { return f.now }}).Update(
			t.Context(),
			f.owner.ID,
			f.id,
			row.Revision,
			event.Input{Name: row.Name, Slug: row.Slug, Configuration: c},
		)
		if err != nil {
			t.Fatal(err)
		}
		changed := f.preview(p)
		if changed.State != "draft" || changed.Revision == p.Revision {
			t.Fatal("date edit retained approval")
		}
		f.now = original
		f.dispatch(p)
		if f.row().PublishedAt.Valid {
			t.Fatal("stale schedule published")
		}
		in := input("challenge")
		p = f.approve(f.save(changed, in))
		f.now = *p.Document.ScheduledAt
		if err = f.s.Maintain(t.Context()); err != nil {
			t.Fatal(err)
		}
		if !f.row().PublishedAt.Valid {
			t.Fatal("new schedule did not publish")
		}
	})
	t.Run("published challenge persists through approved postponement", func(t *testing.T) {
		f := newFixture(t, pool)
		p := f.approve(f.save(content.Page{}, input("challenge")))
		f.now = f.due
		f.dispatch(p)
		published := f.row().PublishedAt
		c := f.c
		c.LiveStart = c.LiveStart.AddDate(0, 0, 7)
		c.LiveEnd = c.LiveEnd.AddDate(0, 0, 7)
		c.SubmissionCutoff = c.SubmissionCutoff.AddDate(0, 0, 7)
		row := f.row()
		s := event.Service{Pool: pool, Clock: func() time.Time { return f.now }}
		in := event.Input{Name: row.Name, Slug: row.Slug, Configuration: c}
		_, err := s.Update(t.Context(), f.owner.ID, f.id, row.Revision, in)
		expectBlocked(t, err)
		in.ApprovePostponement = true
		in.Reason = "Venue disruption. Teams may keep working."
		if _, err = s.Update(t.Context(), f.owner.ID, f.id, row.Revision, in); err != nil {
			t.Fatal(err)
		}
		if !f.row().PublishedAt.Time.Equal(published.Time) {
			t.Fatal("challenge retracted or replayed")
		}
		if _, err = f.s.Read(t.Context(), f.member.ID, f.id, p.ID, false, 0); err != nil {
			t.Fatal(err)
		}
		notices, err := f.s.Notices(t.Context(), f.member.ID, f.id, 0)
		if err != nil || len(notices) != 1 ||
			!notices[0].SubmissionCutoff.Equal(c.SubmissionCutoff) {
			t.Fatalf("revised dates not available: %+v %v", notices, err)
		}
		if _, err = f.s.Notices(
			t.Context(),
			f.other.ID,
			f.id,
			0,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal("notices exposed to outsiders")
		}
		current := f.preview(p)
		_, err = f.s.Action(
			t.Context(),
			f.owner.ID,
			f.id,
			p.ID,
			current.Revision,
			"withdraw",
			"Postponed",
		)
		expectBlocked(t, err)
		correction := input("challenge")
		correction.Document.Markdown = "Corrected common brief"
		current = f.act(f.approve(f.save(current, correction)), "release")
		if !f.row().PublishedAt.Time.Equal(published.Time) || current.Version != 2 {
			t.Fatal("correction replayed common publication")
		}
	})
}

func TestEventOverviewHonoursChallengeDispatchAllowance(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	for _, offset := range []time.Duration{0, 60 * time.Second, 60*time.Second + time.Nanosecond} {
		f.now = f.due.Add(offset)
		v, err := (event.Service{Pool: pool, Clock: func() time.Time { return f.now }}).Read(
			t.Context(),
			f.owner.ID,
			f.id,
		)
		if err != nil {
			t.Fatal(err)
		}
		missed := false
		for _, b := range v.Blockers {
			if b.Code == "publication_missed" {
				missed = true
			}
		}
		if missed != (offset > 60*time.Second) {
			t.Fatalf("at offset %s overview missed=%t", offset, missed)
		}
	}
}

func TestAuthoringReadinessMetadataAndAttachmentIntegrity(t *testing.T) {
	pool := testdb.New(t, true)
	t.Run("readiness is current and organiser only", func(t *testing.T) {
		f := newFixture(t, pool)
		status, err := f.s.PublicationStatus(t.Context(), f.owner.ID, f.id)
		if err != nil || len(status.Blockers) != 0 || status.PublishedAt != nil {
			t.Fatalf("ready status %+v %v", status, err)
		}
		f.exec("UPDATE event_gate_facts SET frozen_teams=0 WHERE event_id=$1", f.id)
		status, err = f.s.PublicationStatus(t.Context(), f.owner.ID, f.id)
		if err != nil || len(status.Blockers) == 0 || status.Blockers[0].Code != "teams_unready" {
			t.Fatalf("stale readiness %+v %v", status, err)
		}
		if _, err = f.s.PublicationStatus(t.Context(), f.member.ID, f.id); !errors.Is(err, access.ErrForbidden) {
			t.Fatal("attendee obtained authoring gates")
		}
	})
	t.Run("missing attachment blocks approval", func(t *testing.T) {
		f := newFixture(t, pool)
		id := f.upload()
		in := input("page")
		in.Document.Attachments = []uuid.UUID{id}
		p := f.act(f.save(content.Page{}, in), "review")
		if len(p.AttachmentDetails) != 1 || p.AttachmentDetails[0].Filename != "notes.txt" || !p.AttachmentDetails[0].CanDownload {
			t.Fatalf("attachment description %+v", p.AttachmentDetails)
		}
		file, err := f.q.File(t.Context(), dbgen.FileParams{EventID: f.id, ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if err = f.fs.Storage.Remove(file.ObjectKey); err != nil {
			t.Fatal(err)
		}
		_, err = f.s.Action(t.Context(), f.owner.ID, f.id, p.ID, p.Revision, "approve", "")
		expectBlocked(t, err)
		if f.preview(p).State != "review" {
			t.Fatal("missing attachment was approved")
		}
	})
	t.Run("attachment lost after approval blocks scheduled release", func(t *testing.T) {
		f := newFixture(t, pool)
		id := f.upload()
		in := input("page")
		in.Document.Attachments = []uuid.UUID{id}
		due := f.now.Add(time.Minute)
		in.Document.ScheduledAt = &due
		p := f.approve(f.save(content.Page{}, in))
		file, err := f.q.File(t.Context(), dbgen.FileParams{EventID: f.id, ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if err = f.fs.Storage.Remove(file.ObjectKey); err != nil {
			t.Fatal(err)
		}
		f.now = due
		f.dispatch(p)
		if f.preview(p).State != "blocked" {
			t.Fatal("missing attachment was released")
		}
	})
}
