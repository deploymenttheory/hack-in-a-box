// Package repositoryaccess owns reviewed collaborator changes and their durable
// external-effect history. It never changes competitor assignments.
package repositoryaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const HandlerKind = "repository.access"

type App interface {
	InspectCollaborators(context.Context, *dbgen.Queries, uuid.UUID, githubapp.AccessRequest) (githubapp.AccessSnapshot, error)
	ChangeCollaborator(context.Context, *dbgen.Queries, uuid.UUID, githubapp.AccessRequest, githubapp.AccessChange) (githubapp.AccessResult, error)
}
type Service struct {
	Pool *pgxpool.Pool
	App  App
	Now  func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

type Member struct {
	PersonID     uuid.UUID `json:"person_id"`
	GitHubID     int64     `json:"github_id"`
	LinkRevision int64     `json:"link_revision"`
	Permission   string    `json:"permission"`
	Role         string    `json:"role"`
}
type Flag struct {
	Code     string     `json:"code"`
	PersonID *uuid.UUID `json:"person_id"`
	GitHubID *int64     `json:"github_id"`
}
type Action struct {
	PreviousState string     `json:"previous_state"`
	ID            uuid.UUID  `json:"id"`
	Kind          string     `json:"kind"`
	Member        Member     `json:"member"`
	GrantID       *uuid.UUID `json:"grant_id"`
}
type Plan struct {
	ID           uuid.UUID  `json:"id"`
	Version      int64      `json:"version"`
	EventID      uuid.UUID  `json:"event_id"`
	TeamID       uuid.UUID  `json:"team_id"`
	RepositoryID int64      `json:"repository_id"`
	Fingerprint  string     `json:"fingerprint"`
	Members      []Member   `json:"members"`
	Actions      []Action   `json:"actions"`
	Flags        []Flag     `json:"flags"`
	CreatedAt    time.Time  `json:"created_at"`
	ApprovedAt   *time.Time `json:"approved_at"`
	ApprovedBy   *uuid.UUID `json:"approved_by"`
}
type Grant struct {
	Uncertain    bool       `json:"effect_uncertain"`
	ID           uuid.UUID  `json:"id"`
	Member       Member     `json:"member"`
	Managed      bool       `json:"app_managed"`
	State        string     `json:"state"`
	InvitationID *int64     `json:"invitation_id"`
	Diagnostic   string     `json:"diagnostic"`
	ObservedAt   *time.Time `json:"observed_at"`
}
type Execution struct {
	ActionID   uuid.UUID `json:"action_id"`
	State      string    `json:"state"`
	Diagnostic string    `json:"diagnostic"`
	Attempted  bool      `json:"attempted"`
	UpdatedAt  time.Time `json:"updated_at"`
}
type View struct {
	Execution  []Execution `json:"execution"`
	Revision   int64       `json:"revision"`
	TeamID     uuid.UUID   `json:"team_id"`
	Plan       *Plan       `json:"plan"`
	Grants     []Grant     `json:"grants"`
	CanManage  bool        `json:"can_manage"`
	GrantsOpen bool        `json:"grants_open"`
	Flags      []Flag      `json:"flags"`
	ObservedAt *time.Time  `json:"observed_at"`
}

func fingerprint(repositoryID int64, provenance uuid.UUID, members []Member) string {
	value, _ := json.Marshal(struct {
		RepositoryID int64
		Provenance   uuid.UUID
		Members      []Member
	}{repositoryID, provenance, members})
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
func permissionName(value string) string {
	if value == "push" {
		return "write"
	}
	if value == "pull" {
		return "read"
	}
	return value
}

func proposalFingerprint(actions []Action, flags []Flag) string {
	normalized := slices.Clone(actions)
	for i := range normalized {
		normalized[i].ID = uuid.Nil
	}
	sorted := slices.Clone(flags)
	slices.SortFunc(sorted, func(a, b Flag) int {
		return strings.Compare(fmt.Sprint(a.Code, a.PersonID, a.GitHubID), fmt.Sprint(b.Code, b.PersonID, b.GitHubID))
	})
	value, _ := json.Marshal(struct {
		Actions []Action
		Flags   []Flag
	}{normalized, sorted})
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
