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

func TestExistingPoolTuningAndEquivalentKeywordDeploymentRemainSupported(t *testing.T) {
	primary := `host=db.example.test port=5432 user=operator password='synthetic \'pass' dbname='event data' sslmode=require pool_max_conns=3`
	equivalent := "postgresql://operator:synthetic%20%27pass@db.example.test:5432/event%20data?sslmode=require"
	actual, err := operatorConnection(primary, equivalent)
	if err != nil || actual != equivalent {
		t.Fatal("ordinary existing keyword DSN rejected with equivalent operator configuration")
	}
	for _, other := range []string{
		"postgresql://operator:synthetic%20%27pass@other.example.test:5432/event%20data?sslmode=require",
		"postgresql://operator:wrong@db.example.test:5432/event%20data?sslmode=require",
		"postgresql://operator:synthetic%20%27pass@db.example.test:5432/event%20data?sslmode=disable",
		"postgresql://operator:synthetic%20%27pass@db.example.test:5432/other?sslmode=require",
	} {
		if _, err = operatorConnection(primary, other); !errors.Is(err, ErrConnection) {
			t.Fatal("changed database/credential/TLS accepted as equivalent")
		}
	}
	environment, err := postgresEnvironment(
		equivalent + "&pool_max_conns=3&pool_min_conns=1&pool_max_conn_lifetime=1h",
	)
	if err != nil {
		t.Fatal("valid pgx pool-only parameters narrowed deployment compatibility")
	}
	for _, entry := range environment {
		if strings.Contains(entry, "pool_") {
			t.Fatal("pool-only tuning leaked into PostgreSQL client settings")
		}
	}
	if _, err = postgresEnvironment(
		equivalent + "&pool_max_conns=invalid",
	); !errors.Is(
		err,
		ErrConnection,
	) {
		t.Fatal("invalid pool tuning silently ignored")
	}
}
