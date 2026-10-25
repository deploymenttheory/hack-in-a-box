package qualification

import (
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestQualificationHistoryRejectsTruncateIncludingCascade(t *testing.T) {
	pool := testdb.New(t, true)
	for _, statement := range []string{"TRUNCATE mentor_qualification_sources", "TRUNCATE mentor_qualification_rounds CASCADE"} {
		if _, err := pool.Exec(t.Context(), statement); err == nil {
			t.Fatalf("immutable qualification history accepted %s", statement)
		}
	}
}
