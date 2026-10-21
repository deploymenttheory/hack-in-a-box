package testdb

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
)

func Person(t *testing.T, pool *pgxpool.Pool) dbgen.Person {
	t.Helper()
	p, err := dbgen.New(pool).
		UpsertIdentity(t.Context(), dbgen.UpsertIdentityParams{Issuer: "synthetic", Subject: uuid.NewString(), DisplayName: "Synthetic person"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func Event(t *testing.T, pool *pgxpool.Pool, owner uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := pool.Exec(
		t.Context(),
		"INSERT INTO events(id,name) VALUES ($1,'Synthetic event')",
		id,
	); err != nil {
		t.Fatal(err)
	}
	if err := permissions.AssignOwner(t.Context(), dbgen.New(pool), id, owner); err != nil {
		t.Fatal(err)
	}
	return id
}
