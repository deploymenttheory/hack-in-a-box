package surveys

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// NotificationSource is routing metadata from an actual published, open survey.
// It contains no response, author, address or free-text question/answer prose.
type NotificationSource struct {
	ID      uuid.UUID
	Version int64
	OpensAt time.Time
}

func (s Service) DiscoverNotificationSourcesPageInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	sourceID *uuid.UUID,
	after uuid.UUID,
	limit int32,
) ([]NotificationSource, *uuid.UUID, error) {
	out := []NotificationSource{}
	if limit < 1 || limit > 200 {
		return out, nil, access.ErrInvalid
	}
	q := dbgen.New(tx)
	e, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return out, nil, fmt.Errorf("lock survey discovery: %w", err)
	}
	if !e.WritesAllowed {
		return out, nil, access.ErrForbidden
	}
	if err = s.privateCurrent(ctx, q, e); err != nil {
		return out, nil, err
	}
	rows, err := q.SurveyOpenSourcesPage(
		ctx,
		dbgen.SurveyOpenSourcesPageParams{
			EventID:  eventID,
			SourceID: sourceID,
			AfterID:  after,
			AtTime:   s.now(),
			PageSize: limit,
		},
	)
	if err != nil {
		return out, nil, fmt.Errorf("page native survey openings: %w", err)
	}
	for _, row := range rows {
		var d Definition
		if err = json.Unmarshal(row.Document, &d); err != nil {
			return out, nil, fmt.Errorf("decode native survey opening: %w", err)
		}
		approval, readErr := q.SurveyApproval(
			ctx,
			dbgen.SurveyApprovalParams{
				EventID:  eventID,
				SurveyID: row.SurveyID,
				Version:  row.Version,
			},
		)
		if readErr != nil {
			return out, nil, fmt.Errorf("read opening approval: %w", readErr)
		}
		if authorize(ctx, q, approval.ActorID, eventID) != nil {
			continue
		}
		out = append(
			out,
			NotificationSource{ID: row.SurveyID, Version: row.Version, OpensAt: d.OpensAt},
		)
	}
	var next *uuid.UUID
	if len(rows) == int(limit) {
		last := rows[len(rows)-1].SurveyID
		next = &last
	}
	return out, next, nil
}

// ResolveInvitationAudienceInTransaction is execution authority, not historical
// read authority. Close/cancel/answer/retention/current-role changes stop sends.
func (s Service) ResolveInvitationAudienceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, id uuid.UUID,
	version int64,
) ([]uuid.UUID, error) {
	q := dbgen.New(tx)
	e, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("lock survey invitation source: %w", err)
	}
	if !e.WritesAllowed {
		return nil, access.ErrForbidden
	}
	if err = s.privateCurrent(ctx, q, e); err != nil {
		return nil, err
	}
	head, cfg, err := current(ctx, q, eventID, id)
	if err != nil {
		return nil, err
	}
	if head.Cancelled || !head.PublishedVersion.Valid || head.PublishedVersion.Int64 != version ||
		s.now().Before(cfg.Definition.OpensAt) ||
		!s.now().Before(cfg.Definition.ClosesAt) {
		return nil, access.ErrForbidden
	}
	approval, err := q.SurveyApproval(
		ctx,
		dbgen.SurveyApprovalParams{EventID: eventID, SurveyID: id, Version: version},
	)
	if err != nil {
		return nil, fmt.Errorf("read current invitation approval: %w", err)
	}
	if err = authorize(ctx, q, approval.ActorID, eventID); err != nil {
		return nil, err
	}
	people, err := s.people(ctx, q, eventID, uuid.Nil, cfg.Definition.Form.Audience)
	if err != nil {
		return nil, err
	}
	eligible := map[uuid.UUID]bool{}
	for _, person := range people {
		eligible[person] = true
	}
	responses, err := q.CurrentSurveyResponses(
		ctx,
		dbgen.CurrentSurveyResponsesParams{EventID: eventID, SurveyID: id},
	)
	if err != nil {
		return nil, fmt.Errorf("read current survey completion: %w", err)
	}
	for _, response := range responses {
		delete(eligible, response.PersonID)
	}
	invites, err := q.SurveyInvitations(
		ctx,
		dbgen.SurveyInvitationsParams{EventID: eventID, SurveyID: id},
	)
	if err != nil {
		return nil, fmt.Errorf("read fixed survey invitations: %w", err)
	}
	if len(invites) > 10000 || len(responses) > 10000 {
		return nil, access.ErrInvalid
	}
	out := []uuid.UUID{}
	for _, invite := range invites {
		if invite.Version == version && eligible[invite.PersonID] {
			out = append(out, invite.PersonID)
		}
	}
	return out, nil
}

// AuthorizeNotificationInTransaction only proves the current person's native
// reader scope over the retained invitation. Answering and close stop execution
// but preserve legitimate own history; withdrawal/expiry still revoke reads.
func (s Service) AuthorizeNotificationInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, id uuid.UUID,
	version int64,
) error {
	if version < 1 {
		return access.ErrNotFound
	}
	q := dbgen.New(tx)
	e, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("lock survey notification reader: %w", err)
	}
	if err = s.privateCurrent(ctx, q, e); err != nil {
		return err
	}
	head, _, err := s.invited(ctx, q, actor, eventID, id)
	if err != nil {
		return err
	}
	if !head.PublishedVersion.Valid || head.PublishedVersion.Int64 != version {
		return access.ErrNotFound
	}
	return nil
}
