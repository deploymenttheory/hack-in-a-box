package rubric

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// QualificationCriteriaInTransaction returns only the actual current published
// mentor-final criteria. It grants no reader/panel permission and does not start
// a run; its consuming selection owner must authorise its own actor.
func QualificationCriteriaInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, runID uuid.UUID,
) (Reference, PublicDefinition, error) {
	var ref Reference
	var out PublicDefinition
	if err := ValidateRunPublicationInTransaction(ctx, tx, eventID, runID); err != nil {
		return ref, out, fmt.Errorf("qualification criteria: %w", err)
	}
	return qualificationCriteria(ctx, tx, eventID, runID)
}

// QualificationHistoryCriteriaInTransaction permits retained-run history only
// while its exact native common publication remains current and readable. It
// does not authorize a reader, restore a run, or grant scoring permission.
func QualificationHistoryCriteriaInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, runID uuid.UUID,
) (Reference, PublicDefinition, bool, error) {
	q := dbgen.New(tx)
	run, err := q.RubricRun(ctx, dbgen.RubricRunParams{EventID: eventID, ID: runID})
	if err != nil {
		return Reference{}, PublicDefinition{}, false, fmt.Errorf(
			"historical qualification run: %w",
			err,
		)
	}
	if err = currentRunPublication(ctx, q, eventID, run); err != nil {
		return Reference{}, PublicDefinition{}, false, err
	}
	ref, criteria, err := qualificationCriteria(ctx, tx, eventID, runID)
	return ref, criteria, run.Status == "current", err
}

func qualificationCriteria(
	ctx context.Context,
	tx pgx.Tx,
	eventID, runID uuid.UUID,
) (Reference, PublicDefinition, error) {
	var ref Reference
	var out PublicDefinition
	q := dbgen.New(tx)
	run, err := q.RubricRun(ctx, dbgen.RubricRunParams{EventID: eventID, ID: runID})
	if err != nil {
		return ref, out, fmt.Errorf("qualification criteria: %w", err)
	}
	if run.Purpose != "mentor_final" {
		return ref, out, access.ErrForbidden
	}
	version, err := q.RubricVersion(
		ctx,
		dbgen.RubricVersionParams{
			EventID:  eventID,
			RubricID: run.RubricID,
			Version:  run.RubricVersion,
		},
	)
	if err != nil {
		return ref, out, fmt.Errorf("qualification criteria: %w", err)
	}
	if err = json.Unmarshal(version.PublicDocument, &out); err != nil {
		return ref, out, fmt.Errorf("decode qualification criteria: %w", err)
	}
	if out.Mode != MentorQualification {
		return ref, out, access.ErrForbidden
	}
	return Reference{
		ID:      run.RubricID,
		Version: run.RubricVersion,
		Digest:  version.PublicDigest,
	}, out, nil
}
