package githubwebhooks

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type Activity struct {
	TeamID              uuid.UUID  `json:"team_id"`
	Pushes              int64      `json:"pushes"`
	AcceptedInvitations int64      `json:"accepted_invitations"`
	LastProviderAt      *time.Time `json:"last_provider_at"`
	LastReceivedAt      time.Time  `json:"last_received_at"`
}
type DeliveryStatus struct {
	ID           uuid.UUID  `json:"id"`
	TeamID       uuid.UUID  `json:"team_id"`
	RepositoryID int64      `json:"repository_id"`
	Kind         string     `json:"kind"`
	Action       string     `json:"action"`
	Ref          string     `json:"ref"`
	AfterSHA     string     `json:"after_sha"`
	ProviderAt   *time.Time `json:"provider_at"`
	ReceivedAt   time.Time  `json:"received_at"`
	Disposition  string     `json:"disposition"`
	OperationID  *uuid.UUID `json:"operation_id"`
	State        string     `json:"state"`
	Attempts     int32      `json:"attempts"`
	Approved     bool       `json:"approved"`
	Cancelled    bool       `json:"cancelled"`
}
type Status struct {
	Informational bool             `json:"informational"`
	Activity      []Activity       `json:"activity"`
	Deliveries    []DeliveryStatus `json:"deliveries"`
}

func timestamp(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func (s Service) Status(ctx context.Context, actor, eventID uuid.UUID) (Status, error) {
	result := Status{Informational: true, Activity: []Activity{}, Deliveries: []DeliveryStatus{}}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, eventID); err != nil {
			return access.ErrNotFound
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.StaffRead,
		); err != nil {
			return fmt.Errorf("authorise repository activity read: %w", err)
		}
		activity, err := q.GitHubRepositoryActivity(ctx, eventID)
		if err != nil {
			return database.ErrDatabase
		}
		for _, a := range activity {
			result.Activity = append(
				result.Activity,
				Activity{
					TeamID:              a.TeamID,
					Pushes:              a.Pushes,
					AcceptedInvitations: a.Invitations,
					LastProviderAt:      timestamp(a.LastProviderAt),
					LastReceivedAt:      a.LastReceivedAt,
				},
			)
		}
		deliveries, err := q.ListGitHubWebhookDeliveries(ctx, eventID)
		if err != nil {
			return database.ErrDatabase
		}
		for _, d := range deliveries {
			result.Deliveries = append(
				result.Deliveries,
				DeliveryStatus{
					ID:           d.ID,
					TeamID:       d.TeamID,
					RepositoryID: d.RepositoryID,
					Kind:         d.Kind,
					Action:       d.Action,
					Ref:          d.Ref,
					AfterSHA:     d.AfterSha,
					ProviderAt:   timestamp(d.ProviderAt),
					ReceivedAt:   d.ReceivedAt,
					Disposition:  d.Disposition,
					OperationID:  d.OperationID,
					State:        d.OperationState,
					Attempts:     d.Attempts,
					Approved:     d.Approved,
					Cancelled:    d.Cancelled,
				},
			)
		}
		return nil
	})
	if err != nil {
		return Status{}, fmt.Errorf("read GitHub operational activity: %w", err)
	}
	return result, nil
}
