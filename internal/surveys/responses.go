package surveys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
)

func (s Service) privateCurrent(ctx context.Context, q *dbgen.Queries, row dbgen.Event) error {
	if row.CompletedAt.Valid &&
		slices.Contains([]string{"completed", "cancelled", "archived"}, row.Stage) {
		policy, err := q.PrivacyPolicy(ctx, row.ID)
		if err != nil {
			return fmt.Errorf("read survey retention policy: %w", err)
		}
		if !s.now().Before(row.CompletedAt.Time.AddDate(0, 0, int(policy.RetentionDays))) {
			return access.ErrForbidden
		}
	}
	return nil
}

func (s Service) invited(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, id uuid.UUID,
) (dbgen.SurveyHead, Configuration, error) {
	head, cfg, err := current(ctx, q, eventID, id)
	if err != nil {
		return head, cfg, err
	}
	if !head.PublishedVersion.Valid {
		return head, cfg, access.ErrNotFound
	}
	invite, err := q.SurveyInvitation(
		ctx,
		dbgen.SurveyInvitationParams{EventID: eventID, SurveyID: id, PersonID: actor},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return head, cfg, access.ErrForbidden
	}
	if err != nil {
		return head, cfg, fmt.Errorf("read own survey invitation: %w", err)
	}
	if invite.Version != head.PublishedVersion.Int64 {
		return head, cfg, access.ErrForbidden
	}
	people, err := s.people(ctx, q, eventID, actor, cfg.Definition.Form.Audience)
	if err != nil {
		return head, cfg, err
	}
	if len(people) != 1 || people[0] != actor {
		return head, cfg, access.ErrForbidden
	}
	return head, cfg, nil
}

func decodeResponse(row dbgen.SurveyResponse) (Response, error) {
	out := Response{
		Revision:    row.Revision,
		FormVersion: row.FormVersion,
		SubmittedAt: row.SubmittedAt,
		Answers:     forms.Answers{},
	}
	if err := json.Unmarshal(row.Answers, &out.Answers); err != nil {
		return out, fmt.Errorf("decode retained survey response: %w", err)
	}
	return out, nil
}

func ownResponse(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, id, person uuid.UUID,
) (*Response, error) {
	row, err := q.SurveyResponse(
		ctx,
		dbgen.SurveyResponseParams{EventID: eventID, SurveyID: id, PersonID: person},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read own survey response: %w", err)
	}
	out, err := decodeResponse(row)
	return &out, err
}

func (s Service) ReadOwn(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
) (ParticipantSurvey, error) {
	var out ParticipantSurvey
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock survey reader: %w", err)
		}
		if err = s.privateCurrent(ctx, q, e); err != nil {
			return err
		}
		head, cfg, err := s.invited(ctx, q, actor, eventID, id)
		if err != nil {
			return err
		}
		response, err := ownResponse(ctx, q, eventID, id, actor)
		if err != nil {
			return err
		}
		out = ParticipantSurvey{
			ID:                 id,
			Version:            cfg.Version,
			Form:               cfg.Definition.Form,
			OpensAt:            cfg.Definition.OpensAt,
			ClosesAt:           cfg.Definition.ClosesAt,
			ResponseVisibility: ResponseVisibility,
			Measures:           cfg.Definition.Measures,
			CanRespond: e.WritesAllowed && !head.Cancelled &&
				!s.now().Before(cfg.Definition.OpensAt) &&
				s.now().Before(cfg.Definition.ClosesAt),
			Response: response,
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("read invited survey: %w", err)
	}
	return out, nil
}

func responseDigest(c ResponseCommand, expected int64) (string, error) {
	body, err := json.Marshal(struct {
		Command  ResponseCommand
		Expected int64
	}{c, expected})
	if err != nil {
		return "", fmt.Errorf("digest survey command: %w", err)
	}
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:]), nil
}

func (s Service) Respond(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	expected int64,
	c ResponseCommand,
) (Response, error) {
	var out Response
	if expected < 0 || c.CommandID == uuid.Nil || c.FormVersion < 1 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock survey response: %w", err)
		}
		if !e.WritesAllowed {
			return access.ErrForbidden
		}
		if err = s.privateCurrent(ctx, q, e); err != nil {
			return err
		}
		head, cfg, err := s.invited(ctx, q, actor, eventID, id)
		if err != nil {
			return err
		}
		if head.Cancelled || s.now().Before(cfg.Definition.OpensAt) ||
			!s.now().Before(cfg.Definition.ClosesAt) {
			return access.ErrForbidden
		}
		if c.FormVersion != cfg.Version {
			return access.ErrConflict
		}
		checked := forms.Check(cfg.Definition.Form, c.Answers)
		if len(checked.Issues) > 0 {
			return &forms.Invalid{Issues: checked.Issues}
		}
		c.Answers = checked.Answers
		digest, err := responseDigest(c, expected)
		if err != nil {
			return err
		}
		previous, err := q.SurveyCommand(
			ctx,
			dbgen.SurveyCommandParams{
				EventID:   eventID,
				SurveyID:  id,
				PersonID:  actor,
				CommandID: c.CommandID,
			},
		)
		if err == nil {
			if previous.PayloadDigest != digest {
				return access.ErrConflict
			}
			r, readErr := ownResponse(ctx, q, eventID, id, actor)
			if readErr != nil {
				return readErr
			}
			if r == nil {
				return access.ErrConflict
			}
			out = *r
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read survey command identity: %w", err)
		}
		before, err := ownResponse(ctx, q, eventID, id, actor)
		if err != nil {
			return err
		}
		version := int64(0)
		if before != nil {
			version = before.Revision
		}
		if version != expected {
			return access.ErrConflict
		}
		version++
		if err = q.SetSurveyResponseHead(
			ctx,
			dbgen.SetSurveyResponseHeadParams{
				EventID:  eventID,
				SurveyID: id,
				PersonID: actor,
				Revision: version,
			},
		); err != nil {
			return fmt.Errorf("retain current response revision: %w", err)
		}
		body, err := json.Marshal(checked.Answers)
		if err != nil {
			return fmt.Errorf("encode survey answers: %w", err)
		}
		if err = q.InsertSurveyResponse(
			ctx,
			dbgen.InsertSurveyResponseParams{
				EventID:     eventID,
				SurveyID:    id,
				PersonID:    actor,
				Revision:    version,
				FormVersion: cfg.Version,
				Answers:     body,
				SubmittedAt: s.now(),
			},
		); err != nil {
			return fmt.Errorf("retain authored survey response: %w", err)
		}
		if err = q.InsertSurveyCommand(
			ctx,
			dbgen.InsertSurveyCommandParams{
				EventID:        eventID,
				SurveyID:       id,
				PersonID:       actor,
				CommandID:      c.CommandID,
				PayloadDigest:  digest,
				ResultRevision: version,
			},
		); err != nil {
			return fmt.Errorf("retain response command identity: %w", err)
		}
		if err = record(ctx, q, actor, eventID, id, audit.SurveyResponded, expected); err != nil {
			return err
		}
		out = Response{
			Revision:    version,
			FormVersion: cfg.Version,
			Answers:     checked.Answers,
			SubmittedAt: s.now(),
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("save survey response: %w", err)
	}
	return out, nil
}

type AuthoredResponse struct {
	PersonID   uuid.UUID `json:"person_id"`
	PersonName string    `json:"person_name"`
	Response   Response  `json:"response"`
}

type ResponsePage struct {
	Items              []AuthoredResponse `json:"items"`
	NextPerson         *uuid.UUID         `json:"next_person"`
	ResponseVisibility string             `json:"response_visibility"`
}

func (s Service) Responses(
	ctx context.Context,
	actor, eventID, id, after uuid.UUID,
	limit int32,
) (ResponsePage, error) {
	out := ResponsePage{Items: []AuthoredResponse{}, ResponseVisibility: ResponseVisibility}
	if limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock authored survey history: %w", err)
		}
		if err = authorize(ctx, q, actor, eventID); err != nil {
			return err
		}
		if err = s.privateCurrent(ctx, q, e); err != nil {
			return err
		}
		if _, _, err = current(ctx, q, eventID, id); err != nil {
			return err
		}
		rows, err := q.SurveyResponseHeadsPage(
			ctx,
			dbgen.SurveyResponseHeadsPageParams{
				EventID:     eventID,
				SurveyID:    id,
				AfterPerson: after,
				PageSize:    limit + 1,
			},
		)
		if err != nil {
			return fmt.Errorf("page authored responses: %w", err)
		}
		if len(rows) > int(limit) {
			last := rows[limit-1].PersonID
			out.NextPerson = &last
			rows = rows[:limit]
		}
		for _, row := range rows {
			r, readErr := ownResponse(ctx, q, eventID, id, row.PersonID)
			if readErr != nil {
				return readErr
			}
			if r == nil {
				continue
			}
			person, readErr := q.Person(ctx, row.PersonID)
			if readErr != nil {
				return fmt.Errorf("read response author: %w", readErr)
			}
			out.Items = append(
				out.Items,
				AuthoredResponse{
					PersonID:   row.PersonID,
					PersonName: person.DisplayName,
					Response:   *r,
				},
			)
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("read authored survey responses: %w", err)
	}
	return out, nil
}
