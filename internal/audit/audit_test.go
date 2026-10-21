package audit_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestAuditRollbackScopeAndSafeMetadata(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	person, err := q.UpsertIdentity(
		ctx,
		dbgen.UpsertIdentityParams{Issuer: "test", Subject: "admin"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.BootstrapAdmin(ctx, person.ID); err != nil {
		t.Fatal(err)
	}
	first, second := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{first, second} {
		if _, err := pool.Exec(
			ctx,
			"INSERT INTO events(id,name) VALUES ($1,'Synthetic')",
			id,
		); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{
			EventID:  first,
			PersonID: person.ID,
			Role:     "organiser",
			Active:   true,
		},
	); err != nil {
		t.Fatal(err)
	}
	record := audit.Record{
		ActorID:  &person.ID,
		EventID:  &first,
		TargetID: first,
		Action:   audit.TeamFrozen,
		Reason:   audit.OrganiserDecision,
	}
	failure := errors.New("simulated rollback")
	if err := database.Within(ctx, pool, func(_ pgx.Tx, queries *dbgen.Queries) error {
		if err := audit.Append(ctx, queries, record); err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	service := audit.Service{Queries: q, Policy: access.EventPolicy{Queries: q}}
	rows, err := service.List(ctx, person.ID, first, 0, 50, false)
	if err != nil || len(rows) != 0 {
		t.Fatalf("audit escaped rollback: %v %v", rows, err)
	}
	if err := audit.Append(ctx, q, record); err != nil {
		t.Fatal(err)
	}
	record.EventID = &second
	if err := audit.Append(ctx, q, record); err != nil {
		t.Fatal(err)
	}
	rows, err = service.List(ctx, person.ID, first, 0, 50, true)
	if err != nil || len(rows) != 1 || *rows[0].EventID != first {
		t.Fatalf("event scope: %v %v", rows, err)
	}
	record.Reason = audit.Reason("secret accessibility or assessment text")
	if err := audit.Append(ctx, q, record); !errors.Is(err, access.ErrInvalid) {
		t.Fatalf("free text accepted: %v", err)
	}
	record.Reason = audit.OrganiserDecision
	record.Action = audit.Action("token-secret")
	if err := audit.Append(ctx, q, record); !errors.Is(err, access.ErrInvalid) {
		t.Fatalf("unknown action accepted: %v", err)
	}
	if strings.Contains(string(rows[0].Metadata), "secret") {
		t.Fatal("unsafe metadata")
	}
	if _, err := pool.Exec(ctx, "UPDATE audit_events SET reason_code='changed'"); err == nil {
		t.Fatal("audit history was editable")
	}
	if _, err := service.List(
		ctx,
		uuid.New(),
		first,
		0,
		50,
		false,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("unauthorized query")
	}
}
