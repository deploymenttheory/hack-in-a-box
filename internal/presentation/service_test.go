package presentation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	t                *testing.T
	pool             *pgxpool.Pool
	service          Service
	owner, mc, other dbgen.Person
	id               uuid.UUID
	now              time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		t:    t,
		pool: testdb.New(t, true),
		now:  time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC),
	}
	f.owner, f.mc, f.other = testdb.Person(
		t,
		f.pool,
	), testdb.Person(
		t,
		f.pool,
	), testdb.Person(
		t,
		f.pool,
	)
	f.id = testdb.Event(t, f.pool, f.owner.ID)
	f.service = Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mc')",
		f.id,
		f.mc.ID,
	)
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) change(actor uuid.UUID, c Command) Page {
	f.t.Helper()
	p, err := f.service.Read(f.t.Context(), actor, f.id)
	if err != nil {
		f.t.Fatal(err)
	}
	if c.Reason == "" {
		c.Reason = "Approved synthetic room update"
	}
	p, err = f.service.Change(f.t.Context(), actor, f.id, p.Revision, c)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *fixture) pair(actor uuid.UUID) Paired {
	f.t.Helper()
	p, err := f.service.Read(f.t.Context(), actor, f.id)
	if err != nil {
		f.t.Fatal(err)
	}
	v, err := f.service.Pair(
		f.t.Context(),
		actor,
		f.id,
		p.Revision,
		PairInput{Label: "Synthetic room", Minutes: 60, Reason: "Approved room pairing"},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *fixture) snapshot(token string) Snapshot {
	f.t.Helper()
	p, err := f.service.Snapshot(f.t.Context(), f.id, token)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *fixture) program(cues []Cue) {
	f.t.Helper()
	f.change(f.owner.ID, Command{Action: "save", Cues: cues})
	f.change(f.owner.ID, Command{Action: "publish"})
}

func TestPresentationDraftApprovalTimersAndConcurrentControllers(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	pair := f.pair(f.mc.ID)
	first := Cue{
		ID:              uuid.New(),
		Title:           "Approved opening",
		Text:            "Welcome to the room",
		URL:             "https://example.invalid/event",
		DurationSeconds: 120,
	}
	second := Cue{ID: uuid.New(), Title: "Next public activity", DurationSeconds: 60}
	f.change(f.owner.ID, Command{Action: "save", Cues: []Cue{first, second}})
	if p := f.snapshot(pair.Token); p.Current != nil || p.Next != nil {
		t.Fatal("unapproved cue reached display")
	}
	f.change(f.mc.ID, Command{Action: "publish"})
	p := f.snapshot(pair.Token)
	if p.Current != nil || p.Next == nil || p.Next.Title != first.Title {
		t.Fatalf("publish must wait for explicit advance: %+v", p)
	}
	f.change(f.mc.ID, Command{Action: "advance"})
	p = f.snapshot(pair.Token)
	if p.Current == nil || p.Current.Title != first.Title || p.Next == nil ||
		p.Next.Title != second.Title ||
		p.RemainingSeconds != 120 ||
		p.TimerEndsAt == nil {
		t.Fatalf("current/next timer: %+v", p)
	}
	f.now = f.now.Add(13 * time.Second)
	if p = f.snapshot(pair.Token); p.RemainingSeconds != 107 || !p.ServerNow.Equal(f.now) {
		t.Fatalf("timer is not server-clock based: %+v", p)
	}
	held := f.change(f.owner.ID, Command{Action: "hold"})
	f.now = f.now.Add(20 * time.Second)
	if p = f.snapshot(
		pair.Token,
	); p.RemainingSeconds != 107 || p.TimerEndsAt != nil ||
		p.Mode != "held" {
		t.Fatalf("hold advanced timer: %+v", p)
	}
	f.change(f.mc.ID, Command{Action: "resume"})
	f.now = f.now.Add(time.Second)
	if p = f.snapshot(pair.Token); p.RemainingSeconds != 106 {
		t.Fatalf("resume restarted wrong duration: %+v", p)
	}
	// Both MCs reviewed one revision. Only one command can commit.
	page, err := f.service.Read(ctx, f.mc.ID, f.id)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	start := make(chan struct{})
	for _, actor := range []uuid.UUID{f.owner.ID, f.mc.ID} {
		go func() {
			<-start
			_, err := f.service.Change(
				ctx,
				actor,
				f.id,
				page.Revision,
				Command{Action: "advance", Reason: "Reviewed next cue"},
			)
			results <- err
		}()
	}
	close(start)
	success, conflict := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, access.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("MC race: %d successes / %d conflicts", success, conflict)
	}
	if _, err = f.service.Change(
		ctx,
		f.mc.ID,
		f.id,
		held.Revision,
		Command{Action: "reset", Reason: "Stale attempt"},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatalf("stale change: %v", err)
	}
	// Unpublished edits never replace the approved current program.
	first.Title = "Private draft replacement"
	f.change(f.owner.ID, Command{Action: "save", Cues: []Cue{first}})
	p = f.snapshot(pair.Token)
	if p.Current == nil || p.Current.Title != second.Title {
		t.Fatal("draft edit replaced the live program")
	}
}

func TestDisplayTokensAreScopedHashedRevocableAndAuthorityBound(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	pair := f.pair(f.mc.ID)
	foreign := testdb.Event(t, f.pool, f.owner.ID)
	if _, err := f.service.Snapshot(
		ctx,
		foreign,
		pair.Token,
	); !errors.Is(
		err,
		access.ErrUnauthenticated,
	) {
		t.Fatalf("cross-event display: %v", err)
	}
	for _, secret := range []string{"", strings.Repeat("x", 52)} {
		if _, err := f.service.Snapshot(
			ctx,
			f.id,
			secret,
		); !errors.Is(
			err,
			access.ErrUnauthenticated,
		) {
			t.Fatalf("invalid capability: %v", err)
		}
	}
	f.exec("UPDATE people SET platform_admin=true WHERE id=$1", f.other.ID)
	if _, err := f.service.Read(ctx, f.other.ID, f.id); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("platform admin inherited MC access: %v", err)
	}
	var hash []byte
	if err := f.pool.QueryRow(ctx, "SELECT token_hash FROM display_pairings WHERE event_id=$1 AND id=$2", f.id, pair.Pairing.ID).
		Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if len(hash) != 32 || string(hash) == pair.Token {
		t.Fatal("plaintext display secret stored")
	}
	page, err := f.service.Read(ctx, f.owner.ID, f.id)
	if err != nil {
		t.Fatal(err)
	}
	serialized, _ := json.Marshal(page)
	if strings.Contains(string(serialized), pair.Token) ||
		strings.Contains(string(serialized), "token_hash") {
		t.Fatal("pairing secret reappeared in controller view")
	}
	if _, err = f.service.Revoke(
		ctx,
		f.owner.ID,
		f.id,
		pair.Pairing.ID,
		page.Revision,
		"Display lost",
	); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Snapshot(
		ctx,
		f.id,
		pair.Token,
	); !errors.Is(
		err,
		access.ErrUnauthenticated,
	) {
		t.Fatalf("revoked display remained active: %v", err)
	}
	pair = f.pair(f.mc.ID)
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mc'",
		f.id,
		f.mc.ID,
	)
	if _, err = f.service.Snapshot(
		ctx,
		f.id,
		pair.Token,
	); !errors.Is(
		err,
		access.ErrUnauthenticated,
	) {
		t.Fatalf("revoked issuer remained active: %v", err)
	}
	if _, err = f.service.Read(ctx, f.mc.ID, f.id); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("revoked controller read: %v", err)
	}
	pair = f.pair(f.owner.ID)
	f.now = f.now.Add(time.Hour)
	if _, err = f.service.Snapshot(
		ctx,
		f.id,
		pair.Token,
	); !errors.Is(
		err,
		access.ErrUnauthenticated,
	) {
		t.Fatalf("expired display remained active: %v", err)
	}
}

func TestDisplayProjectionRequiresReleasedCommonContentAndOmitsPrivateFields(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	pair := f.pair(f.mc.ID)
	source := content.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	input := content.Input{
		Kind: "page",
		Document: content.Document{
			Title:       "Released room title",
			Markdown:    "Private body not projected",
			Audience:    access.ContentAudience{Confirmed: true},
			Attachments: []uuid.UUID{},
		},
	}
	id := uuid.New()
	page, err := source.Save(ctx, f.owner.ID, f.id, id, 0, input)
	if err != nil {
		t.Fatal(err)
	}
	cue := Cue{
		ID:        uuid.New(),
		Title:     "Approved cue with source",
		Text:      "Approved room copy",
		ContentID: &id,
	}
	f.program([]Cue{cue})
	f.change(f.mc.ID, Command{Action: "advance"})
	if p := f.snapshot(pair.Token); p.Current != nil {
		t.Fatal("unreleased source disclosed its cue")
	}
	for _, action := range []string{"review", "approve", "release"} {
		page, err = source.Action(
			ctx,
			f.owner.ID,
			f.id,
			id,
			page.Revision,
			action,
			"Approved public room source",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	p := f.snapshot(pair.Token)
	sequence := p.Sequence
	if p.Current == nil || p.Current.ContentTitle != "Released room title" {
		t.Fatalf("released source withheld: %+v", p)
	}
	// Editing a later private draft cannot replace its last released version.
	input.Document.Title = "Unreleased replacement secret"
	page, err = source.Save(ctx, f.owner.ID, f.id, id, page.Revision, input)
	if err != nil {
		t.Fatal(err)
	}
	p = f.snapshot(pair.Token)
	data, _ := json.Marshal(p)
	for _, secret := range []string{"Private body not projected", "Unreleased replacement secret", pair.Token, "issuer_id", "draft", "pairings", "actor_id", "token_hash"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("private field leaked: %s", secret)
		}
	}
	if p.Current == nil || p.Current.ContentTitle != "Released room title" {
		t.Fatal("latest draft replaced released source title")
	}
	if _, err = source.Action(
		ctx,
		f.owner.ID,
		f.id,
		id,
		page.Revision,
		"withdraw",
		"Withdraw room source",
	); err != nil {
		t.Fatal(err)
	}
	if p = f.snapshot(pair.Token); p.Current != nil || p.Sequence != sequence {
		t.Fatal("withdrawal must update the projection without needing a cue-control revision")
	}
	// A released organiser-only page must not be converted to room access.
	privateID := uuid.New()
	input.Document.Title = "Organiser-only title"
	input.Document.Audience = access.ContentAudience{Roles: []string{"organiser"}}
	private, err := source.Save(ctx, f.owner.ID, f.id, privateID, 0, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve", "release"} {
		private, err = source.Action(
			ctx,
			f.owner.ID,
			f.id,
			privateID,
			private.Revision,
			action,
			"Synthetic private approval",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	cue.ContentID = &privateID
	f.program([]Cue{cue})
	f.change(f.mc.ID, Command{Action: "advance"})
	if p = f.snapshot(pair.Token); p.Current != nil || p.Next != nil {
		t.Fatal("private-role source reached display")
	}
	cue.ContentID = &uuid.Nil
	if _, err = f.service.Change(
		ctx,
		f.owner.ID,
		f.id,
		f.change(f.owner.ID, Command{Action: "hold"}).Revision,
		Command{Action: "save", Reason: "Invalid source", Cues: []Cue{cue}},
	); !errors.Is(
		err,
		access.ErrInvalid,
	) {
		t.Fatalf("invalid source identity accepted: %v", err)
	}
}

func TestDisplayWithholdsForeignTeamPrivateAndEarlyChallengeSources(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	paired := f.pair(f.mc.ID)
	source := content.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	released := func(eventID uuid.UUID, audience access.ContentAudience) uuid.UUID {
		t.Helper()
		id := uuid.New()
		p, err := source.Save(
			ctx,
			f.owner.ID,
			eventID,
			id,
			0,
			content.Input{
				Kind: "page",
				Document: content.Document{
					Title:    "Restricted source title",
					Markdown: "Not a room body",
					Audience: audience,
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		for _, action := range []string{"review", "approve", "release"} {
			p, err = source.Action(
				ctx,
				f.owner.ID,
				eventID,
				id,
				p.Revision,
				action,
				"Synthetic content approval",
			)
			if err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	foreignEvent := testdb.Event(t, f.pool, f.owner.ID)
	foreign := released(foreignEvent, access.ContentAudience{Confirmed: true})
	team, err := dbgen.New(f.pool).
		CreateHackTeam(ctx, dbgen.CreateHackTeamParams{EventID: f.id, ID: uuid.New(), Name: "Private team"})
	if err != nil {
		t.Fatal(err)
	}
	private := released(f.id, access.ContentAudience{Teams: []uuid.UUID{team.ID}})
	c := event.DefaultConfiguration()
	c.LiveStart = f.now.AddDate(0, 0, 8)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.TeamFreeze = f.now.Add(12 * time.Hour)
	c.AdmissionCutoff = c.TeamFreeze.Add(-time.Hour)
	c.SubmissionCutoff = c.LiveStart.Add(6 * time.Hour)
	data, _ := json.Marshal(c)
	f.exec("UPDATE events SET configuration=$2 WHERE id=$1", f.id, data)
	challengeID := uuid.New()
	if _, err = source.Save(
		ctx,
		f.owner.ID,
		f.id,
		challengeID,
		0,
		content.Input{
			Kind: "challenge",
			Document: content.Document{
				Title:    "Early challenge secret",
				Markdown: "Not published yet",
				Audience: access.ContentAudience{Confirmed: true},
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{foreign, private, challengeID} {
		f.program([]Cue{{ID: uuid.New(), Title: "Linked cue must remain withheld", ContentID: &id}})
		f.change(f.mc.ID, Command{Action: "advance"})
		p := f.snapshot(paired.Token)
		if p.Current != nil || p.Next != nil {
			t.Fatalf("restricted source %s reached a paired display: %+v", id, p)
		}
	}
}
