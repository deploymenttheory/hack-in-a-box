package rubric

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// ReleasedSource identifies the actual common owning release. It carries neither
// private judge guidance nor a draft/latest substitute for its exact rubric.
type ReleasedSource struct {
	ChallengeID      uuid.UUID
	ChallengeVersion int64
	Reference        Reference
}

// ReleasedSourceForReaderInTransaction supplies native question/answer workflows
// with the exact source currently readable by a confirmed attendee or organiser.
// It locks the event in the caller's transaction; the caller must independently
// enforce team-specific access and write/archive policy for its own resource.
func ReleasedSourceForReaderInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	now time.Time,
) (ReleasedSource, error) {
	q := dbgen.New(tx)
	e, err := q.LockEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReleasedSource{}, access.ErrNotFound
	}
	if err != nil {
		return ReleasedSource{}, fmt.Errorf("lock released rubric source: %w", err)
	}
	roles, confirmed, err := access.ContentReader(ctx, q, actor, eventID)
	if err != nil {
		return ReleasedSource{}, fmt.Errorf("ReleasedSourceForReaderInTransaction: %w", err)
	}
	if (!confirmed && !slices.Contains(roles, "organiser")) ||
		!e.PublishedAt.Valid || now.Before(e.PublishedAt.Time) {
		return ReleasedSource{}, access.ErrNotFound
	}
	challenge, err := q.RubricChallenge(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReleasedSource{}, access.ErrNotFound
	}
	if err != nil {
		return ReleasedSource{}, fmt.Errorf("read released challenge source: %w", err)
	}
	version, err := access.ContentVersionForReader(ctx, q, actor, challenge, false)
	if err != nil {
		return ReleasedSource{}, fmt.Errorf("ReleasedSourceForReaderInTransaction: %w", err)
	}
	if version == 0 || !challenge.ReleasedAt.Valid || now.Before(challenge.ReleasedAt.Time) {
		return ReleasedSource{}, access.ErrNotFound
	}
	page, err := releasedSnapshot(ctx, q, eventID, challenge.ID, version)
	if err != nil {
		return ReleasedSource{}, err
	}
	if now.Before(page.PublishedAt) {
		return ReleasedSource{}, access.ErrNotFound
	}
	return ReleasedSource{
		ChallengeID:      challenge.ID,
		ChallengeVersion: version,
		Reference:        page.Reference,
	}, nil
}

// releasedSnapshot projects the exact owning publication after the caller's
// current reader/actual-release checks. All public consumers share its digest and
// challenge-reference validation; private judge instructions are never decoded.
func releasedSnapshot(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, challengeID uuid.UUID,
	version int64,
) (PublicPage, error) {
	publication, err := q.RubricPublication(
		ctx,
		dbgen.RubricPublicationParams{
			EventID:          eventID,
			ChallengeID:      challengeID,
			ChallengeVersion: version,
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicPage{}, access.ErrNotFound
	}
	if err != nil {
		return PublicPage{}, fmt.Errorf("read released rubric publication: %w", err)
	}
	content, err := q.ContentVersion(
		ctx,
		dbgen.ContentVersionParams{EventID: eventID, ContentID: challengeID, Version: version},
	)
	if err != nil {
		return PublicPage{}, fmt.Errorf("read released challenge reference: %w", err)
	}
	var document struct {
		Rubric *Reference `json:"rubric"`
	}
	if err = json.Unmarshal(content.Document, &document); err != nil {
		return PublicPage{}, fmt.Errorf("decode common rubric reference: %w", err)
	}
	ref := document.Rubric
	if ref == nil || ref.ID != publication.RubricID || ref.Version != publication.Version ||
		ref.Digest != publication.PublicDigest {
		return PublicPage{}, access.ErrConflict
	}
	rubric, err := q.RubricVersion(
		ctx,
		dbgen.RubricVersionParams{EventID: eventID, RubricID: ref.ID, Version: ref.Version},
	)
	if err != nil {
		return PublicPage{}, fmt.Errorf("read released common rubric: %w", err)
	}
	var definition PublicDefinition
	if err = json.Unmarshal(rubric.PublicDocument, &definition); err != nil {
		return PublicPage{}, fmt.Errorf("decode released common criteria: %w", err)
	}
	if (Definition{PublicDefinition: definition}).PublicDigest() != ref.Digest {
		return PublicPage{}, access.ErrConflict
	}
	return PublicPage{
		Reference:   *ref,
		Definition:  definition,
		PublishedAt: publication.PublishedAt,
		Notice:      publication.ReviewedNotice,
	}, nil
}
