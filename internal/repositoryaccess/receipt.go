package repositoryaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// Receipt identifies an original retained command, independently of the latest
// roster, plan, provider observation and event revision. A nil Plan proves no
// outcome either way and must not authorise replay or replacement.
type Receipt struct {
	CommandFingerprint *string    `json:"command_fingerprint"`
	Action             string     `json:"action"`
	ActorID            *uuid.UUID `json:"actor_id"`
	Plan               *Plan      `json:"plan"`
}

// ReadReceipt is a current-authorised, actor-bound lookup only. It does not call
// the provider, derive a plan, audit a write, enqueue work or advance revisions.
func (s Service) ReadReceipt(ctx context.Context, actor, eventID, teamID, planID uuid.UUID, action string) (Receipt, error) {
	out := Receipt{Action: action}
	if planID == uuid.Nil || (action != "review" && action != "approve") {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, eventID); err != nil {
			return wrap("lock original repository access receipt", err)
		}
		if err := authorize(ctx, q, actor, eventID, false); err != nil {
			return err
		}
		current, err := repository(ctx, q, eventID, teamID)
		if err != nil {
			return err
		}
		var encoded []byte
		var createdBy uuid.UUID
		var approvedBy *uuid.UUID
		var approvedAt *time.Time
		var approvalReason string
		err = tx.QueryRow(ctx, `SELECT definition,created_by,approved_by,approved_at,approval_reason FROM repository_access_plans WHERE id=$1 AND event_id=$2 AND team_id=$3`, planID, eventID, teamID).Scan(&encoded, &createdBy, &approvedBy, &approvedAt, &approvalReason)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read original repository access receipt: %w", err)
		}
		originalActor := createdBy
		if action == "approve" {
			if approvedBy == nil || approvedAt == nil {
				return nil
			}
			originalActor = *approvedBy
		}
		// Do not reveal another actor's command body, even to another organiser.
		if originalActor != actor {
			return nil
		}
		var retained Plan
		if err = json.Unmarshal(encoded, &retained); err != nil {
			return fmt.Errorf("decode original repository access receipt: %w", err)
		}
		if retained.ID != planID || retained.EventID != eventID || retained.TeamID != teamID || retained.RepositoryID != current.GithubRepositoryID.Int64 {
			return access.ErrConflict
		}
		// A review receipt is the immutable original definition, not a later approval.
		retained.ApprovedAt = nil
		retained.ApprovedBy = nil
		retained.ApprovalReason = ""
		if action == "approve" {
			retained.ApprovedAt = approvedAt
			retained.ApprovedBy = approvedBy
			retained.ApprovalReason = approvalReason
		}
		out.ActorID = &originalActor
		out.Plan = &retained
		fingerprint := commandFingerprint(action, originalActor, retained)
		out.CommandFingerprint = &fingerprint
		return nil
	})
	return out, wrap("original repository access receipt", err)
}

// UTF-8 length framing avoids JSON HTML/Unicode escaping differences across
// Go and browser implementations. Review's command version is natively zero;
// approval binds the retained plan version. This hashes no provider credentials.
func commandFingerprint(action string, actor uuid.UUID, plan Plan) string {
	version, reason := int64(0), plan.Reason
	if action == "approve" {
		version, reason = plan.Version, plan.ApprovalReason
	}
	fields := []string{action, actor.String(), plan.EventID.String(), plan.TeamID.String(), strconv.FormatInt(plan.RepositoryID, 10), plan.ID.String(), strconv.FormatInt(version, 10), strings.TrimSpace(reason)}
	var canonical strings.Builder
	canonical.WriteString("hackbox:repository-access-command:v1\n")
	for _, field := range fields {
		canonical.WriteString(strconv.Itoa(len(field)))
		canonical.WriteByte(':')
		canonical.WriteString(field)
	}
	sum := sha256.Sum256([]byte(canonical.String()))
	return hex.EncodeToString(sum[:])
}
