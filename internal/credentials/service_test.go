package credentials

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	t                    *testing.T
	pool                 *pgxpool.Pool
	s                    Service
	owner, member, other dbgen.Person
	event, team          uuid.UUID
}

func keyring(t *testing.T, active string, ids ...string) *secretbox.Keyring {
	t.Helper()
	keys := map[string]string{}
	for i, id := range ids {
		keys[id] = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{byte(i + 1)}, 32))
	}
	raw, _ := json.Marshal(map[string]any{"active": active, "keys": keys})
	r, e := secretbox.Parse(string(raw))
	if e != nil {
		t.Fatal(e)
	}
	return r
}

func setup(t *testing.T) *fixture {
	return setupWithPool(t, testdb.New(t, true))
}

func setupWithPool(t *testing.T, p *pgxpool.Pool) *fixture {
	t.Helper()
	f := &fixture{
		t:      t,
		pool:   p,
		owner:  testdb.Person(t, p),
		member: testdb.Person(t, p),
		other:  testdb.Person(t, p),
		team:   uuid.New(),
	}
	f.event = testdb.Event(t, p, f.owner.ID)
	f.s = Service{Pool: p, Keys: keyring(t, "old", "old")}
	f.exec(
		"INSERT INTO hack_teams(id,event_id,name,state,frozen) VALUES($1,$2,'Synthetic team','published',true)",
		f.team,
		f.event,
	)
	f.exec(
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		f.event,
		f.member.ID,
	)
	f.exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		f.event,
		f.team,
		f.member.ID,
	)
	if e := dbgen.New(p).
		CreateCompetitionEntry(t.Context(), dbgen.CreateCompetitionEntryParams{EventID: f.event, TeamID: f.team}); e != nil {
		t.Fatal(e)
	}
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, e := f.pool.Exec(f.t.Context(), sql, args...); e != nil {
		f.t.Fatal(e)
	}
}

func (f *fixture) revision() int64 {
	f.t.Helper()
	r, e := dbgen.New(f.pool).Event(f.t.Context(), f.event)
	if e != nil {
		f.t.Fatal(e)
	}
	return r.Revision
}

func (f *fixture) change(c Command) Page {
	f.t.Helper()
	p, e := f.s.Change(f.t.Context(), f.owner.ID, f.event, f.revision(), c)
	if e != nil {
		f.t.Fatal(e)
	}
	return p
}

func (f *fixture) reveal(id uuid.UUID) (Revealed, error) {
	return f.s.Reveal(f.t.Context(), f.member.ID, f.event, id, f.revision())
}

func TestCredentialCurrentMembershipPrivacyAndAuditedDenials(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	p := f.change(
		Command{
			Action:  "create",
			Label:   "Synthetic service",
			Value:   "SYNTHETIC-SECRET",
			TeamIDs: []uuid.UUID{f.team},
		},
	)
	id := p.Items[0].ID
	encoded, _ := json.Marshal(p)
	if strings.Contains(string(encoded), "SYNTHETIC-SECRET") ||
		strings.Contains(string(encoded), "ciphertext") ||
		strings.Contains(string(encoded), "key_id") {
		t.Fatal("unsafe metadata")
	}
	v, e := f.reveal(id)
	if e != nil || v.Value != "SYNTHETIC-SECRET" {
		t.Fatalf("reveal: %v", e)
	}
	for _, actor := range []uuid.UUID{f.owner.ID, f.other.ID} {
		if _, e = f.s.Reveal(
			ctx,
			actor,
			f.event,
			id,
			f.revision(),
		); !errors.Is(
			e,
			access.ErrForbidden,
		) {
			t.Fatalf("unexpected organiser/outsider reveal: %v", e)
		}
	}
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.event,
		f.member.ID,
	)
	if _, e = f.reveal(id); !errors.Is(e, access.ErrForbidden) {
		t.Fatalf("withdrawn authority: %v", e)
	}
	f.exec(
		"UPDATE competition_claims SET state='confirmed' WHERE event_id=$1 AND person_id=$2",
		f.event,
		f.member.ID,
	)
	f.exec("DELETE FROM hack_team_members WHERE event_id=$1 AND person_id=$2", f.event, f.member.ID)
	if _, e = f.reveal(id); !errors.Is(e, access.ErrForbidden) {
		t.Fatalf("removed member authority: %v", e)
	}
	f.exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		f.event,
		f.team,
		f.member.ID,
	)
	f.change(Command{Action: "assign", ID: id, TeamIDs: []uuid.UUID{}})
	if _, e = f.reveal(id); !errors.Is(e, access.ErrForbidden) {
		t.Fatalf("revoked audience: %v", e)
	}
	f.change(Command{Action: "assign", ID: id, TeamIDs: []uuid.UUID{f.team}})
	f.change(Command{Action: "revoke", ID: id})
	if _, e = f.reveal(id); !errors.Is(e, access.ErrForbidden) {
		t.Fatalf("revoked resource: %v", e)
	}
	rows, e := f.pool.Query(
		ctx,
		"SELECT metadata::text FROM audit_events WHERE action='credential.revealed' AND target_id=$1 ORDER BY sequence",
		id,
	)
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var data string
		if e = rows.Scan(&data); e != nil {
			t.Fatal(e)
		}
		n++
		if strings.Contains(data, "SECRET") ||
			(!strings.Contains(data, "success") && !strings.Contains(data, "denied")) {
			t.Fatalf("unsafe result: %s", data)
		}
	}
	if rows.Err() != nil || n != 7 {
		t.Fatalf("reveal attempts lost: %d %v", n, rows.Err())
	}
}

func TestCredentialKeyFaultRotationReplacementAndSafeRecovery(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	p := f.change(
		Command{
			Action:  "create",
			Label:   "Synthetic",
			Value:   "FIRST-SYNTHETIC",
			TeamIDs: []uuid.UUID{f.team},
		},
	)
	id := p.Items[0].ID
	stored, e := dbgen.New(f.pool).
		TeamCredential(ctx, dbgen.TeamCredentialParams{EventID: f.event, ID: id})
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(stored.Ciphertext, []byte("FIRST-SYNTHETIC")) {
		t.Fatal("plaintext at rest")
	}
	f.s.Keys = nil
	if _, e = f.reveal(id); !errors.Is(e, secretbox.ErrUnavailable) {
		t.Fatal(e)
	}
	f.s.Keys = keyring(t, "wrong", "wrong")
	if _, e = f.reveal(id); !errors.Is(e, secretbox.ErrUnavailable) {
		t.Fatal(e)
	}
	f.s.Keys = keyring(t, "old", "old")
	tampered := bytes.Clone(stored.Ciphertext)
	tampered[len(tampered)-1] ^= 1
	f.exec("UPDATE team_credentials SET ciphertext=$2 WHERE id=$1", id, tampered)
	if _, e = f.reveal(id); !errors.Is(e, secretbox.ErrUnavailable) {
		t.Fatal(e)
	}
	before := f.revision()
	if _, e = f.s.Change(
		ctx,
		f.owner.ID,
		f.event,
		before,
		Command{Action: "rotate", ID: id},
	); !errors.Is(e, secretbox.ErrUnavailable) ||
		f.revision() != before {
		t.Fatal("failed rotation changed state")
	}
	f.exec("UPDATE team_credentials SET ciphertext=$2 WHERE id=$1", id, stored.Ciphertext)
	f.s.Keys = keyring(t, "new", "old", "new")
	f.change(Command{Action: "rotate", ID: id})
	latest, e := dbgen.New(f.pool).
		TeamCredential(ctx, dbgen.TeamCredentialParams{EventID: f.event, ID: id})
	if e != nil || latest.KeyID != "new" || latest.Version != 2 {
		t.Fatalf("rotation: %v %+v", e, latest)
	}
	// Restore the same new key under its identifier after retiring the old key.
	raw, _ := json.Marshal(
		map[string]any{
			"active": "new",
			"keys": map[string]string{
				"new": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)),
			},
		},
	)
	f.s.Keys, e = secretbox.Parse(string(raw))
	if e != nil {
		t.Fatal(e)
	}
	if v, e := f.reveal(id); e != nil || v.Value != "FIRST-SYNTHETIC" {
		t.Fatalf("rotated recovery: %v", e)
	}
	f.change(
		Command{Action: "replace", ID: id, Label: "Synthetic replaced", Value: "SECOND-SYNTHETIC"},
	)
	if v, e := f.reveal(id); e != nil || v.Value != "SECOND-SYNTHETIC" || v.Version != 3 {
		t.Fatalf("replacement: %v", e)
	}
	f.exec(
		"UPDATE team_credentials SET ciphertext=$2,key_id=$3 WHERE id=$1",
		id,
		stored.Ciphertext,
		stored.KeyID,
	)
	if _, e = f.reveal(id); !errors.Is(e, secretbox.ErrUnavailable) {
		t.Fatalf("old version swap: %v", e)
	}
	var failures int
	if e = f.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE action='credential.revealed' AND metadata->>'result'='unavailable'").
		Scan(&failures); e != nil ||
		failures != 4 {
		t.Fatalf("key failures not committed: %d %v", failures, e)
	}
}

func TestCredentialAudienceNeverFollowsDraftsOrDisbandedEntries(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	p := f.change(
		Command{
			Action:  "create",
			Label:   "Synthetic",
			Value:   "SYNTHETIC",
			TeamIDs: []uuid.UUID{f.team},
		},
	)
	id := p.Items[0].ID
	otherTeam := uuid.New()
	f.exec(
		"INSERT INTO hack_teams(id,event_id,name,state) VALUES($1,$2,'Other synthetic team','published')",
		otherTeam,
		f.event,
	)
	f.exec(
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		f.event,
		f.other.ID,
	)
	f.exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published'),($1,$4,$3,'draft')",
		f.event,
		otherTeam,
		f.other.ID,
		f.team,
	)
	if e := dbgen.New(f.pool).
		CreateCompetitionEntry(ctx, dbgen.CreateCompetitionEntryParams{EventID: f.event, TeamID: otherTeam}); e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.Reveal(
		ctx,
		f.other.ID,
		f.event,
		id,
		f.revision(),
	); !errors.Is(
		e,
		access.ErrForbidden,
	) {
		t.Fatalf("cross team draft access: %v", e)
	}
	page, e := f.s.List(ctx, f.other.ID, f.event)
	if e != nil || len(page.Items) != 0 {
		t.Fatalf("cross team metadata: %+v %v", page, e)
	}
	f.exec(
		"UPDATE competition_entries SET active=false WHERE event_id=$1 AND team_id=$2",
		f.event,
		f.team,
	)
	if _, e = f.reveal(id); !errors.Is(e, access.ErrForbidden) {
		t.Fatalf("retired entry access: %v", e)
	}
	f.exec(
		"UPDATE competition_entries SET active=true WHERE event_id=$1 AND team_id=$2",
		f.event,
		f.team,
	)
	f.exec("UPDATE hack_teams SET state='disbanded' WHERE id=$1", f.team)
	if _, e = f.reveal(id); !errors.Is(e, access.ErrForbidden) {
		t.Fatalf("disbanded team access: %v", e)
	}
	f.exec("UPDATE hack_teams SET state='published' WHERE id=$1", f.team)
	f.exec("UPDATE people SET active=false WHERE id=$1", f.member.ID)
	if _, e = f.reveal(id); !errors.Is(e, access.ErrForbidden) {
		t.Fatalf("inactive person access: %v", e)
	}
}

func TestCredentialAuditFailurePreventsDisclosureAndStaleWrites(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	p := f.change(
		Command{
			Action:  "create",
			Label:   "Synthetic",
			Value:   "SYNTHETIC-AUDIT",
			TeamIDs: []uuid.UUID{f.team},
		},
	)
	id := p.Items[0].ID
	stale := f.revision()
	f.change(Command{Action: "replace", ID: id, Label: "Updated", Value: "UPDATED-SYNTHETIC"})
	if v, e := f.s.Reveal(
		ctx,
		f.member.ID,
		f.event,
		id,
		stale,
	); !errors.Is(e, access.ErrConflict) ||
		v.Value != "" {
		t.Fatalf("stale reveal: %v", e)
	}
	if _, e := f.s.Change(
		ctx,
		f.owner.ID,
		f.event,
		stale,
		Command{Action: "replace", ID: id, Label: "Lost", Value: "LOST-SYNTHETIC"},
	); !errors.Is(
		e,
		access.ErrConflict,
	) {
		t.Fatalf("stale replace: %v", e)
	}
	// A database audit fault must prevent disclosure, even after decryption.
	f.exec(
		"CREATE FUNCTION reject_credential_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='credential.revealed' THEN RAISE EXCEPTION 'synthetic audit outage'; END IF; RETURN NEW; END $$",
	)
	f.exec(
		"CREATE TRIGGER credential_audit_fault BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_credential_audit()",
	)
	if v, e := f.reveal(id); e == nil || v.Value != "" {
		t.Fatal("disclosed without durable audit")
	}
	f.exec("DROP TRIGGER credential_audit_fault ON audit_events")
	if v, e := f.reveal(id); e != nil || v.Value != "UPDATED-SYNTHETIC" {
		t.Fatalf("recovery: %v", e)
	}
}

func TestCredentialConcurrentRotationAndReplacementHaveOneWinner(t *testing.T) {
	f := setup(t)
	p := f.change(
		Command{
			Action:  "create",
			Label:   "Synthetic",
			Value:   "BEFORE-SYNTHETIC",
			TeamIDs: []uuid.UUID{f.team},
		},
	)
	id := p.Items[0].ID
	f.s.Keys = keyring(t, "new", "old", "new")
	revision := f.revision()
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, command := range []Command{{Action: "rotate", ID: id}, {Action: "replace", ID: id, Label: "Replacement", Value: "AFTER-SYNTHETIC"}} {
		go func() {
			<-start
			_, err := f.s.Change(t.Context(), f.owner.ID, f.event, revision, command)
			results <- err
		}()
	}
	close(start)
	successes, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, access.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 || f.revision() != revision+1 {
		t.Fatalf("lost serialization: %d successes, %d conflicts", successes, conflicts)
	}
	value, err := f.reveal(id)
	if err != nil || value.Version != 2 ||
		(value.Value != "BEFORE-SYNTHETIC" && value.Value != "AFTER-SYNTHETIC") {
		t.Fatalf("invalid winning ciphertext: %v", err)
	}
}
