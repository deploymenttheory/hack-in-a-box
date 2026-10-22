package help

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

// NotificationSink receives source references and authoritative recipient IDs,
// never request prose/location. Its implementation belongs to the inbox graph.
type NotificationSink interface {
	RecordHelpInTransaction(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, int64, []uuid.UUID) error
}

type Service struct {
	Pool          *pgxpool.Pool
	Clock         func() time.Time
	Notifications NotificationSink
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC().Truncate(time.Microsecond)
	}
	return time.Now().UTC().Truncate(time.Microsecond)
}

func wrap(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}

func pointer(v pgtype.Timestamptz) *time.Time {
	if v.Valid {
		return &v.Time
	}
	return nil
}

func stamp(v *time.Time) pgtype.Timestamptz {
	if v != nil {
		return pgtype.Timestamptz{Time: *v, Valid: true}
	}
	return pgtype.Timestamptz{}
}

func request(row dbgen.HelpRequest) Request {
	return Request{
		ID:             row.ID,
		EventID:        row.EventID,
		TeamID:         row.TeamID,
		RequesterID:    row.RequesterID,
		Revision:       row.Revision,
		Category:       row.Category,
		Description:    row.Description,
		Location:       row.Location,
		MeetingURL:     row.MeetingUrl,
		Status:         row.Status,
		AssigneeID:     row.AssigneeID,
		WaitingCycle:   row.WaitingCycle,
		RequestedAt:    row.RequestedAt,
		WaitingSince:   row.WaitingSince,
		FirstClaimedAt: pointer(row.FirstClaimedAt),
		ClaimedAt:      pointer(row.ClaimedAt),
		ResolvedAt:     pointer(row.ResolvedAt),
		UpdatedAt:      row.UpdatedAt,
	}
}

func load(ctx context.Context, q *dbgen.Queries, eventID, id uuid.UUID) (Request, error) {
	row, err := q.HelpRequest(ctx, dbgen.HelpRequestParams{EventID: eventID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, access.ErrNotFound
	}
	if err != nil {
		return Request{}, wrap("load help request", err)
	}
	resource, err := q.PolicyResource(ctx, dbgen.PolicyResourceParams{EventID: eventID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, access.ErrNotFound
	}
	if err != nil {
		return Request{}, wrap("load native help resource", err)
	}
	if resource.Kind != "help" {
		return Request{}, access.ErrNotFound
	}
	return request(row), nil
}

func lock(ctx context.Context, q *dbgen.Queries, eventID uuid.UUID) (dbgen.Event, error) {
	row, err := q.LockEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, access.ErrNotFound
	}
	return row, wrap("lock help event", err)
}

func (s Service) authority(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	r Request,
) (Authority, error) {
	a, err := q.HelpAuthority(
		ctx,
		dbgen.HelpAuthorityParams{
			EventID:  r.EventID,
			TeamID:   r.TeamID,
			PersonID: actor,
			Category: r.Category,
			NowAt:    s.now(),
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Authority{}, access.ErrNotFound
	}
	if err != nil {
		return Authority{}, wrap("read current help authority", err)
	}
	if !a.Active {
		return Authority{}, access.ErrNotFound
	}
	return Authority{Organiser: a.Organiser, TeamMember: a.TeamMember, Helper: a.Helper}, nil
}

func (s Service) authorizeRead(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
	r Request,
) (Authority, error) {
	a, err := s.authority(ctx, q, actor, r)
	if err != nil {
		return a, err
	}
	if !a.Organiser && !a.TeamMember && !a.Helper {
		return a, access.ErrNotFound
	}
	var cfg event.Configuration
	if err = json.Unmarshal(row.Configuration, &cfg); err != nil {
		return a, wrap("read help live start", err)
	}
	return a, outputReady(
		r.Category,
		row.PublishedAt.Valid && !row.PublishedAt.Time.After(s.now()),
		cfg.LiveStart,
		s.now(),
	)
}

func settings(ctx context.Context, q *dbgen.Queries, eventID uuid.UUID) (Settings, error) {
	v, err := q.HelpSettings(ctx, eventID)
	return Settings{
		Revision:                  v.Revision,
		SetupThresholdSeconds:     v.SetupThresholdSeconds,
		LogisticsThresholdSeconds: v.LogisticsThresholdSeconds,
		OutputThresholdSeconds:    v.OutputThresholdSeconds,
		MeetingHosts:              v.MeetingHosts,
	}, wrap(
		"read help settings",
		err,
	)
}

func persist(
	ctx context.Context,
	q *dbgen.Queries,
	r Request,
	action string,
	actor *uuid.UUID,
) error {
	if err := q.UpdateHelpRequest(
		ctx,
		dbgen.UpdateHelpRequestParams{
			EventID:        r.EventID,
			ID:             r.ID,
			Revision:       r.Revision,
			Status:         r.Status,
			AssigneeID:     r.AssigneeID,
			WaitingCycle:   r.WaitingCycle,
			WaitingSince:   r.WaitingSince,
			FirstClaimedAt: stamp(r.FirstClaimedAt),
			ClaimedAt:      stamp(r.ClaimedAt),
			ResolvedAt:     stamp(r.ResolvedAt),
			UpdatedAt:      r.UpdatedAt,
		},
	); err != nil {
		return wrap("update help state", err)
	}
	return history(ctx, q, r, action, actor)
}

func history(
	ctx context.Context,
	q *dbgen.Queries,
	r Request,
	action string,
	actor *uuid.UUID,
) error {
	return wrap(
		"record help transition",
		q.InsertHelpTransition(
			ctx,
			dbgen.InsertHelpTransitionParams{
				EventID:    r.EventID,
				RequestID:  r.ID,
				Revision:   r.Revision,
				Action:     action,
				Status:     r.Status,
				ActorID:    actor,
				AssigneeID: r.AssigneeID,
				OccurredAt: r.UpdatedAt,
			},
		),
	)
}

func digest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", wrap("digest help command", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func duplicate(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, actor, commandID, requestID uuid.UUID,
	payload string,
) (bool, error) {
	old, err := q.HelpCommand(
		ctx,
		dbgen.HelpCommandParams{EventID: eventID, ActorID: actor, CommandID: commandID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, wrap("read help command", err)
	}
	if old.RequestID != requestID || old.PayloadDigest != payload {
		return false, access.ErrConflict
	}
	return true, nil
}

func retainCommand(
	ctx context.Context,
	q *dbgen.Queries,
	actor, commandID uuid.UUID,
	payload string,
	r Request,
) error {
	return wrap(
		"retain help command",
		q.InsertHelpCommand(
			ctx,
			dbgen.InsertHelpCommandParams{
				EventID:          r.EventID,
				ActorID:          actor,
				CommandID:        commandID,
				RequestID:        r.ID,
				PayloadDigest:    payload,
				ResponseRevision: r.Revision,
			},
		),
	)
}

// reconcile releases an unavailable assignee under the same event lock used by
// role/assignment writers. Repeated calls append no duplicate transition.
func (s Service) reconcile(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	r Request,
) (Request, bool, error) {
	if !row.WritesAllowed || r.Status != "claimed" || r.AssigneeID == nil {
		return r, false, nil
	}
	a, err := s.authority(ctx, q, *r.AssigneeID, r)
	if err != nil && !errors.Is(err, access.ErrNotFound) {
		return r, false, err
	}
	if err == nil && a.Helper {
		return r, false, nil
	}
	r.Status = "waiting"
	r.AssigneeID = nil
	r.ClaimedAt = nil
	r.WaitingCycle++
	r.WaitingSince = s.now()
	r.UpdatedAt = r.WaitingSince
	r.Revision++
	return r, true, persist(ctx, q, r, "assignee_unavailable", nil)
}

func (s Service) projection(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
	r Request,
) (Page, error) {
	a, err := s.authorizeRead(ctx, q, row, actor, r)
	if err != nil {
		return Page{}, err
	}
	rows, err := q.HelpTransitions(
		ctx,
		dbgen.HelpTransitionsParams{EventID: r.EventID, RequestID: r.ID},
	)
	if err != nil {
		return Page{}, wrap("load help history", err)
	}
	var config event.Configuration
	if err = json.Unmarshal(row.Configuration, &config); err != nil {
		return Page{}, wrap("project help event timezone", err)
	}
	p := Page{
		EventName:       row.Name,
		Timezone:        config.Timezone,
		Request:         r,
		History:         []Transition{},
		TransferTargets: []HelperTarget{},
	}
	p.AssigneeName, err = assigneeName(ctx, q, r)
	if err != nil {
		return p, err
	}
	for _, h := range rows {
		p.History = append(
			p.History,
			Transition{
				Revision:   h.Revision,
				Action:     h.Action,
				Status:     h.Status,
				ActorID:    h.ActorID,
				AssigneeID: h.AssigneeID,
				At:         h.OccurredAt,
			},
		)
	}
	if !row.WritesAllowed {
		return p, nil
	}
	assigned := r.AssigneeID != nil && *r.AssigneeID == actor && a.Helper
	p.CanClaim = r.Status == "waiting" && a.Helper
	p.CanRelease = r.Status == "claimed" && (a.Organiser || assigned)
	p.CanTransfer = p.CanRelease
	p.CanResolve = p.CanRelease
	p.CanReopen = (r.Status == "resolved" || r.Status == "cancelled") &&
		(a.Organiser || a.TeamMember)
	p.CanCancel = (r.Status == "waiting" || r.Status == "claimed") && (a.Organiser || a.TeamMember)
	if p.CanTransfer {
		candidates, candidateErr := q.HelpRecipientCandidates(
			ctx,
			dbgen.HelpRecipientCandidatesParams{EventID: r.EventID, TeamID: r.TeamID},
		)
		if candidateErr != nil {
			return p, wrap("read transfer candidates", candidateErr)
		}
		for _, id := range candidates {
			if r.AssigneeID != nil && id == *r.AssigneeID {
				continue
			}
			eligible, eligibilityErr := s.authority(ctx, q, id, r)
			if errors.Is(eligibilityErr, access.ErrNotFound) {
				continue
			}
			if eligibilityErr != nil {
				return p, eligibilityErr
			}
			if !eligible.Helper {
				continue
			}
			person, personErr := q.Person(ctx, id)
			if personErr != nil {
				return p, wrap("read eligible transfer name", personErr)
			}
			p.TransferTargets = append(
				p.TransferTargets,
				HelperTarget{PersonID: id, Name: person.DisplayName},
			)
		}
	}
	return p, nil
}

// The request is authorised before this projection; names identify only its
// already disclosed current owner, not a browsable staff directory.
func assigneeName(ctx context.Context, q *dbgen.Queries, r Request) (string, error) {
	if r.AssigneeID == nil {
		return "", nil
	}
	p, err := q.Person(ctx, *r.AssigneeID)
	return p.DisplayName, wrap("read current help owner name", err)
}

func (s Service) Read(ctx context.Context, actor, eventID, id uuid.UUID) (Page, error) {
	var p Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, eventID)
		if err != nil {
			return err
		}
		r, err := load(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		if _, err = s.authorizeRead(ctx, q, row, actor, r); err != nil {
			return err
		}
		r, changed, err := s.reconcile(ctx, q, row, r)
		if err != nil {
			return err
		}
		if changed {
			if err = s.emit(ctx, tx, q, row, r); err != nil {
				return err
			}
		}
		p, err = s.projection(ctx, q, row, actor, r)
		return err
	})
	return p, wrap("read help request", err)
}

func (s Service) Create(ctx context.Context, actor, eventID, id uuid.UUID, in Input) (Page, error) {
	var p Page
	payload, err := digest(in)
	if err != nil {
		return p, err
	}
	if id == uuid.Nil {
		return p, access.ErrInvalid
	}
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, eventID)
		if err != nil {
			return err
		}
		r := Request{
			ID:          id,
			EventID:     eventID,
			TeamID:      in.TeamID,
			RequesterID: actor,
			Category:    in.Category,
		}
		a, err := s.authorizeRead(ctx, q, row, actor, r)
		if err != nil {
			return err
		}
		if !a.TeamMember {
			return access.ErrForbidden
		}
		if !row.WritesAllowed {
			return access.ErrForbidden
		}
		config, err := settings(ctx, q, eventID)
		if err != nil {
			return err
		}
		if err = validateInput(in, config); err != nil {
			return err
		}
		retried, err := duplicate(ctx, q, eventID, actor, in.CommandID, id, payload)
		if err != nil {
			return err
		}
		if retried {
			r, err = load(ctx, q, eventID, id)
			if err != nil {
				return err
			}
			p, err = s.projection(ctx, q, row, actor, r)
			return err
		}
		if _, err = q.HelpRequest(
			ctx,
			dbgen.HelpRequestParams{EventID: eventID, ID: id},
		); err == nil {
			return access.ErrConflict
		} else if !errors.Is(
			err,
			pgx.ErrNoRows,
		) {
			return wrap("check help identity", err)
		}
		registered, registerErr := q.RegisterHelpResource(
			ctx,
			dbgen.RegisterHelpResourceParams{EventID: eventID, ID: id},
		)
		if registerErr != nil {
			return wrap("register native help resource", registerErr)
		}
		if registered != 1 {
			return access.ErrConflict
		}
		now := s.now()
		if err = q.InsertHelpRequest(
			ctx,
			dbgen.InsertHelpRequestParams{
				EventID:     eventID,
				ID:          id,
				TeamID:      in.TeamID,
				RequesterID: actor,
				Category:    in.Category,
				Description: in.Description,
				Location:    in.Location,
				MeetingUrl:  in.MeetingURL,
				RequestedAt: now,
			},
		); err != nil {
			return wrap("create help request", err)
		}
		r, err = load(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		if err = history(ctx, q, r, "create", &actor); err != nil {
			return err
		}
		if err = retainCommand(ctx, q, actor, in.CommandID, payload, r); err != nil {
			return err
		}
		if err = s.emit(ctx, tx, q, row, r); err != nil {
			return err
		}
		p, err = s.projection(ctx, q, row, actor, r)
		return err
	})
	return p, wrap("create help request", err)
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	revision int64,
	in Command,
) (Page, error) {
	var p Page
	if in.CommandID == uuid.Nil {
		return p, access.ErrInvalid
	}
	payload, err := digest(in)
	if err != nil {
		return p, err
	}
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, eventID)
		if err != nil {
			return err
		}
		r, err := load(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		a, err := s.authorizeRead(ctx, q, row, actor, r)
		if err != nil {
			return err
		}
		if !row.WritesAllowed {
			return access.ErrForbidden
		}
		retried, err := duplicate(ctx, q, eventID, actor, in.CommandID, id, payload)
		if err != nil {
			return err
		}
		if retried {
			p, err = s.projection(ctx, q, row, actor, r)
			return err
		}
		r, _, err = s.reconcile(ctx, q, row, r)
		if err != nil {
			return err
		}
		if r.Revision != revision {
			return access.ErrConflict
		}
		eligible := false
		if in.AssigneeID != nil {
			target, targetErr := s.authority(ctx, q, *in.AssigneeID, r)
			if targetErr != nil && !errors.Is(targetErr, access.ErrNotFound) {
				return targetErr
			}
			eligible = target.Helper
		}
		r, err = apply(r, actor, in, a, eligible, s.now())
		if err != nil {
			return err
		}
		if err = persist(ctx, q, r, in.Action, &actor); err != nil {
			return err
		}
		if err = retainCommand(ctx, q, actor, in.CommandID, payload, r); err != nil {
			return err
		}
		if err = s.emit(ctx, tx, q, row, r); err != nil {
			return err
		}
		p, err = s.projection(ctx, q, row, actor, r)
		return err
	})
	return p, wrap("change help request", err)
}

func (s Service) recipients(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	r Request,
) ([]uuid.UUID, error) {
	candidates, err := q.HelpRecipientCandidates(
		ctx,
		dbgen.HelpRecipientCandidatesParams{EventID: r.EventID, TeamID: r.TeamID},
	)
	if err != nil {
		return nil, wrap("load help recipients", err)
	}
	people := []uuid.UUID{}
	for _, actor := range candidates {
		if _, err = s.authorizeRead(ctx, q, row, actor, r); err == nil {
			people = append(people, actor)
		} else if !errors.Is(err, access.ErrNotFound) && !errors.Is(err, event.ErrBlocked) {
			return nil, err
		}
	}
	return people, nil
}

func (s Service) emit(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	row dbgen.Event,
	r Request,
) error {
	resourceID := r.ID
	if err := communications.RecordOfficialFact(
		ctx,
		tx,
		dbgen.SaveCommunicationFactParams{
			EventID:    r.EventID,
			ID:         r.ID,
			Kind:       "help",
			OwnerID:    &r.RequesterID,
			ResourceID: &resourceID,
			Status:     factStatus(r),
			Revision:   r.Revision,
		},
	); err != nil {
		return wrap("record actual help routing fact", err)
	}
	if s.Notifications == nil {
		return nil
	}
	recipients, err := s.recipients(ctx, q, row, r)
	if err != nil {
		return err
	}
	return wrap(
		"record safe help notifications",
		s.Notifications.RecordHelpInTransaction(ctx, tx, r.EventID, r.ID, r.Revision, recipients),
	)
}
