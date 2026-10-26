package loadprofile

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
)

// Effects independently reads all sent candidate identities, including native
// non-2xx/transport-unknown candidates. It never repairs data or marks an
// original response acknowledged. Only bounded aggregate reports leave this
// private reconciliation boundary.
func (n NativeDatabase) Effects(
	ctx context.Context,
	specs map[uuid.UUID]WriteSpec,
) (map[uuid.UUID]EffectFact, []Commit, error) {
	facts := make(map[uuid.UUID]EffectFact, len(specs))
	unique := make(map[string]Commit)
	for id, spec := range specs {
		if err := n.scoped([]uuid.UUID{spec.Event}); err != nil {
			return nil, nil, err
		}
		fact, err := n.effect(ctx, spec)
		if err != nil {
			return nil, nil, err
		}
		facts[id] = fact
		if fact.Present && fact.Effects > 0 {
			identity := spec.identity()
			commit := Commit{
				Identity:    identity,
				Fingerprint: fact.Fingerprint,
				Effects:     fact.Effects,
			}
			if previous, exists := unique[identity]; exists &&
				(previous.Fingerprint != commit.Fingerprint || previous.Effects != commit.Effects) {
				return nil, nil, fmt.Errorf(
					"%w: same immutable effect changed during reconciliation",
					errNativeOracle,
				)
			}
			// Exact ACK replay/old rejected candidates can name the same immutable
			// version. Count its actual committed effect once, not once per HTTP.
			unique[identity] = commit
		}
	}
	commits := make([]Commit, 0, len(unique))
	for _, commit := range unique {
		commits = append(commits, commit)
	}
	return facts, commits, nil
}

func (n NativeDatabase) effect(ctx context.Context, spec WriteSpec) (EffectFact, error) {
	fact := EffectFact{Spec: spec}
	var err error
	switch spec.Kind {
	case RegistrationEffect:
		var actor uuid.UUID
		err = n.pool.QueryRow(ctx, `SELECT person_id,request_hash FROM registrations WHERE event_id=$1 AND id=$2`, spec.Event, spec.Resource).
			Scan(&actor, &fact.Fingerprint)
		if err == nil && actor != spec.Actor {
			return fact, fmt.Errorf("%w: registration effect actor mismatch", errNativeOracle)
		}
	case MentorEffect:
		var actor uuid.UUID
		var evidence observations.Evidence
		err = n.pool.QueryRow(ctx, `SELECT author_id,category,evidence,outcome,feedback_reference FROM mentor_observation_versions WHERE event_id=$1 AND team_id=$2 AND id=$3 AND version=$4`, spec.Event, spec.Team, spec.Resource, spec.Version).
			Scan(&actor, &evidence.Category, &evidence.Evidence, &evidence.Outcome, &evidence.FeedbackReference)
		if err == nil {
			if actor != spec.Actor {
				return fact, fmt.Errorf(
					"%w: immutable mentor effect author mismatch",
					errNativeOracle,
				)
			}
			fact.Fingerprint, err = payloadFingerprint(evidence)
		}
	case HelpEffect:
		var request uuid.UUID
		var version int64
		err = n.pool.QueryRow(ctx, `SELECT c.request_id,c.response_revision,c.payload_digest,(SELECT count(*) FROM help_transitions t WHERE t.event_id=c.event_id AND t.request_id=c.request_id AND t.revision=c.response_revision AND t.actor_id=c.actor_id AND t.action='claim' AND t.assignee_id=c.actor_id) FROM help_commands c WHERE c.event_id=$1 AND c.actor_id=$2 AND c.command_id=$3`, spec.Event, spec.Actor, spec.Command).
			Scan(&request, &version, &fact.Fingerprint, &fact.Effects)
		if err == nil && (request != spec.Resource || version != spec.Version) {
			return fact, fmt.Errorf(
				"%w: original help receipt/transition mismatch",
				errNativeOracle,
			)
		}
	default:
		return fact, fmt.Errorf("%w: unsupported immutable effect owner", errNativeOracle)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fact, nil
	}
	if err != nil {
		return fact, fmt.Errorf("read independent immutable native effect: %w", err)
	}
	fact.Present = true
	if spec.Kind != HelpEffect {
		fact.Effects = 1
	}
	return fact, nil
}
