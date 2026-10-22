package testdb

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

// ApprovedRubric prepares an explicit synthetic organiser decision for native
// challenge fixtures. It does not release either the rubric or the challenge.
func ApprovedRubric(t *testing.T, pool *pgxpool.Pool, actor, eventID uuid.UUID) rubric.Reference {
	t.Helper()
	service := rubric.Service{Pool: pool}
	configuration, err := service.Configuration(t.Context(), actor, eventID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Reference.ID == uuid.Nil {
		definition := configuration.Definition
		configuration, err = service.Change(
			t.Context(),
			actor,
			eventID,
			0,
			rubric.Command{
				Action:     "save",
				Definition: &definition,
				Reason:     "Synthetic explicit common rubric",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !configuration.Approved {
		configuration, err = service.Change(
			t.Context(),
			actor,
			eventID,
			configuration.Revision,
			rubric.Command{
				Action:  "approve",
				Version: configuration.Reference.Version,
				Reason:  "Synthetic explicit rubric approval",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	return configuration.Reference
}
