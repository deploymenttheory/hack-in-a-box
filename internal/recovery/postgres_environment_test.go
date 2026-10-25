package recovery

import (
	"errors"
	"strings"
	"testing"
)

func TestPostgresURIBecomesExplicitLibpqEnvironmentWithoutLosingTLSOrEncoding(t *testing.T) {
	environment, err := postgresEnvironment(
		"postgresql://operator:synthetic%40pass%3A%27@db.example.test:5433/event%20data?sslmode=verify-full&sslrootcert=%2Fidentity%2Fdatabase-ca.pem&connect_timeout=9&options=-c%20timezone%3DUTC",
	)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{}
	for _, entry := range environment {
		key, value, _ := strings.Cut(entry, "=")
		fields[key] = value
	}
	expected := map[string]string{
		"PGUSER":            "operator",
		"PGPASSWORD":        "synthetic@pass:'",
		"PGHOST":            "db.example.test",
		"PGPORT":            "5433",
		"PGDATABASE":        "event data",
		"PGSSLMODE":         "verify-full",
		"PGSSLROOTCERT":     "/identity/database-ca.pem",
		"PGCONNECT_TIMEOUT": "9",
		"PGOPTIONS":         "-c timezone=UTC",
	}
	for key, value := range expected {
		if fields[key] != value {
			t.Fatalf("connection field not preserved: %s", key)
		}
	}
	if strings.Contains(fields["PGDATABASE"], "postgresql://") {
		t.Fatal("URI left unexpanded as literal database default")
	}
	for _, invalid := range []string{"host=db user=operator dbname=event", "postgres://operator@db/event?unknown_option=1", "postgres://operator@db/event?sslmode=verify-full&sslmode=disable", "postgres://operator@db/event#fragment"} {
		if _, err = postgresEnvironment(invalid); !errors.Is(err, ErrConnection) {
			t.Fatal("unsupported/ambiguous connection silently accepted")
		}
	}
}
