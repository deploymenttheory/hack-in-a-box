package feedback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func settings(ctx context.Context, q *dbgen.Queries, eventID uuid.UUID) (Settings, error) {
	r, err := q.FeedbackSettings(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{
			Version:          1,
			SetupSeconds:     900,
			LogisticsSeconds: 900,
			OutputSeconds:    1200,
		}, nil
	}
	return Settings{
		Version:          r.Version,
		SetupSeconds:     r.SetupSeconds,
		LogisticsSeconds: r.LogisticsSeconds,
		OutputSeconds:    r.OutputSeconds,
	}, wrap(
		"read feedback follow-up threshold",
		err,
	)
}

func (s Service) Settings(ctx context.Context, actor, eventID uuid.UUID) (Settings, error) {
	var out Settings
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := lock(ctx, q, eventID); err != nil {
			return wrap("authorise feedback settings", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.StaffRead,
		); err != nil {
			return wrap("authorise feedback settings", err)
		}
		var err error
		out, err = settings(ctx, q, eventID)
		return err
	})
	return out, wrap("read feedback settings", err)
}

func (s Service) Configure(
	ctx context.Context,
	actor, eventID uuid.UUID,
	expected int64,
	in Settings,
) (Settings, error) {
	if err := in.validate(); err != nil {
		return Settings{}, err
	}
	var out Settings
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, eventID)
		if err != nil {
			return err
		}
		if !row.WritesAllowed {
			return access.ErrForbidden
		}
		if err = (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		); err != nil {
			return wrap("authorize feedback settings change", err)
		}
		old, err := settings(ctx, q, eventID)
		if err != nil {
			return err
		}
		if old.Version != expected {
			return access.ErrConflict
		}
		out = in
		out.Version = old.Version + 1
		if err = q.InsertFeedbackSettings(
			ctx,
			dbgen.InsertFeedbackSettingsParams{
				EventID:          eventID,
				Version:          out.Version,
				SetupSeconds:     out.SetupSeconds,
				LogisticsSeconds: out.LogisticsSeconds,
				OutputSeconds:    out.OutputSeconds,
				ActorID:          actor,
				ChangedAt:        s.now(),
			},
		); err != nil {
			return wrap("retain feedback settings history", err)
		}
		return record(ctx, q, eventID, actor, eventID, out.Version)
	})
	return out, wrap("configure feedback overdue follow-up", err)
}

func (s Service) overdue(a Action, category string, cfg Settings) bool {
	return (a.Status == "open" || a.Status == "in_progress") &&
		!s.now().Before(a.DueAt.Add(time.Duration(cfg.threshold(category))*time.Second))
}

// Maintain runs independently of campaign bindings. Every event and action is
// keyset-paged; no permanent cap can starve an old overdue support task.
func (s Service) Maintain(ctx context.Context) error {
	after := uuid.Nil
	for {
		events, err := dbgen.New(s.Pool).FeedbackMaintenanceEvents(ctx, after)
		if err != nil {
			return wrap("discover feedback maintenance events", err)
		}
		for _, id := range events {
			if err = database.Within(
				ctx,
				s.Pool,
				func(tx pgx.Tx, _ *dbgen.Queries) error { return s.MaintainInTransaction(ctx, tx, id) },
			); err != nil {
				return wrap("maintain overdue feedback event", err)
			}
			after = id
		}
		if len(events) < 100 {
			return nil
		}
	}
}

func (s Service) MaintainInTransaction(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) error {
	q := dbgen.New(tx)
	row, err := lock(ctx, q, eventID)
	if err != nil {
		return err
	}
	if !row.WritesAllowed {
		return nil
	}
	cfg, err := settings(ctx, q, eventID)
	if err != nil {
		return err
	}
	after := uuid.Nil
	for {
		heads, err := q.FeedbackHeadsPage(
			ctx,
			dbgen.FeedbackHeadsPageParams{EventID: eventID, AfterID: after, PageSize: 100},
		)
		if err != nil {
			return wrap("page follow-up feedback", err)
		}
		for _, h := range heads {
			after = h.ID
			if err = s.maintainFeedback(ctx, tx, q, row, h, cfg); err != nil {
				return err
			}
		}
		if len(heads) < 100 {
			return nil
		}
	}
}

func (s Service) maintainFeedback(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	row dbgen.Event,
	h dbgen.FeedbackHead,
	cfg Settings,
) error {
	if !h.PublishedVersion.Valid {
		return nil
	}
	if err := s.gate(row, h.Category); errors.Is(err, access.ErrForbidden) {
		return nil
	} else if err != nil {
		return err
	}
	active, err := q.HelpTeamActive(ctx, dbgen.HelpTeamActiveParams{EventID: row.ID, ID: h.TeamID})
	if err != nil {
		return wrap("current follow-up team", err)
	}
	if !active {
		return nil
	}
	cursor := uuid.Nil
	for {
		actions, err := q.FeedbackActionHeadsPage(
			ctx,
			dbgen.FeedbackActionHeadsPageParams{
				EventID:    row.ID,
				FeedbackID: h.ID,
				AfterID:    cursor,
				PageSize:   100,
			},
		)
		if err != nil {
			return wrap("page overdue actions", err)
		}
		for _, ah := range actions {
			cursor = ah.ID
			a, err := actionVersion(ctx, q, row.ID, ah.ID, ah.CurrentVersion)
			if err != nil {
				return err
			}
			if err = s.escalateAction(ctx, tx, q, row, h, a, cfg); err != nil {
				return err
			}
		}
		if len(actions) < 100 {
			return nil
		}
	}
}

func (s Service) escalationAuthority(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	h dbgen.FeedbackHead,
) error {
	a, err := s.authority(ctx, q, actor, eventID, h.TeamID, h.Category)
	if err != nil {
		return err
	}
	if a.Organiser {
		return nil
	}
	if h.Category == "competition_output" || !a.Helper {
		return access.ErrNotFound
	}
	roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
	if err != nil {
		return wrap("current escalation category role", err)
	}
	for _, role := range roles {
		if role == "logistics" {
			return nil
		}
	}
	return access.ErrNotFound
}

func (s Service) escalateAction(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	row dbgen.Event,
	h dbgen.FeedbackHead,
	a Action,
	cfg Settings,
) error {
	if !s.overdue(a, h.Category, cfg) {
		return nil
	}
	people, err := q.HelpRecipientCandidates(
		ctx,
		dbgen.HelpRecipientCandidatesParams{EventID: row.ID, TeamID: h.TeamID},
	)
	if err != nil {
		return wrap("derive escalation recipients", err)
	}
	for _, id := range people {
		err = s.escalationAuthority(ctx, q, id, row.ID, h)
		if errors.Is(err, access.ErrNotFound) || errors.Is(err, access.ErrForbidden) {
			continue
		}
		if err != nil {
			return err
		}
		noticeID := uuid.NewSHA1(
			a.ID,
			[]byte(fmt.Sprintf("%s:%d:%d:%s", row.ID, a.Cycle, cfg.Version, id)),
		)
		inserted, err := q.InsertFeedbackEscalation(
			ctx,
			dbgen.InsertFeedbackEscalationParams{
				EventID:          row.ID,
				ID:               noticeID,
				ActionID:         a.ID,
				Cycle:            a.Cycle,
				SettingsVersion:  cfg.Version,
				RecipientID:      id,
				ThresholdSeconds: cfg.threshold(h.Category),
				OccurredAt:       s.now(),
			},
		)
		if err != nil {
			return wrap("deduplicate overdue escalation", err)
		}
		if inserted > 0 && s.Notifications != nil {
			if err = s.Notifications.RecordFeedbackFollowupInTransaction(
				ctx,
				tx,
				row.ID,
				noticeID,
				id,
			); err != nil {
				return wrap("record authorised overdue prompt", err)
			}
		}
	}
	return nil
}
