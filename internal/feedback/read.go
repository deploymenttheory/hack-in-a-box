package feedback

import (
	"context"
	"errors"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func pageLimit(n int32) (int32, error) {
	if n < 1 || n > 100 {
		return 0, access.ErrInvalid
	}
	return n, nil
}

// Published prose is deliberately released. Private provenance never inherits
// that release: every source reference retains its native current owner policy.
func (s Service) projectDocument(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	d Document,
	staff bool,
) Document {
	sources := []Source{}
	if staff {
		for _, ref := range d.Sources {
			if owner := s.Sources[ref.Kind]; owner != nil &&
				owner.AuthorizeInTransaction(ctx, tx, actor, eventID, d.TeamID, ref) == nil {
				sources = append(sources, ref)
			}
		}
	}
	d.Sources = sources
	d.ReviewedDigest = ""
	if d.HelpID != nil {
		owner := s.Sources["help"]
		if owner == nil ||
			owner.AuthorizeInTransaction(
				ctx,
				tx,
				actor,
				eventID,
				d.TeamID,
				Source{Kind: "help", ID: *d.HelpID},
			) != nil {
			d.HelpID = nil
		}
	}
	return d
}

func (s Service) project(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
	h dbgen.FeedbackHead,
) (Page, error) {
	out := Page{Acknowledgements: []Acknowledgement{}, Escalations: []Escalation{}}
	a, err := s.authority(ctx, q, actor, row.ID, h.TeamID, h.Category)
	if err != nil {
		return out, err
	}
	if err = s.gate(row, h.Category); err != nil {
		return out, err
	}
	out.CanManage = a.Organiser || a.Helper
	out.CanAcknowledge = a.TeamMember && row.WritesAllowed && h.PublishedVersion.Valid
	if out.CanManage {
		d, err := document(ctx, q, row.ID, h.ID, h.CurrentVersion)
		if err != nil {
			return out, err
		}
		// Unreleased prose derived from a private source remains private to its
		// current native readers; category helper authority is not a source grant.
		sourceErr := s.sourceAuthority(ctx, tx, actor, row.ID, d)
		if sourceErr == nil {
			d = s.projectDocument(ctx, tx, actor, row.ID, d, true)
			out.Draft = &d
		} else if !errors.Is(sourceErr, access.ErrForbidden) && !errors.Is(sourceErr, access.ErrNotFound) && !errors.Is(sourceErr, access.ErrConflict) {
			return out, sourceErr
		}
		out.CurrentVersion = h.CurrentVersion
	}
	if h.PublishedVersion.Valid {
		d, err := document(ctx, q, row.ID, h.ID, h.PublishedVersion.Int64)
		if err != nil {
			return out, err
		}
		if d.State != "published" {
			return out, access.ErrNotFound
		}
		d = s.projectDocument(ctx, tx, actor, row.ID, d, out.CanManage)
		out.Published = &d
		if !out.CanManage {
			out.CurrentVersion = d.Version
		}
		rows, err := q.FeedbackAcknowledgements(
			ctx,
			dbgen.FeedbackAcknowledgementsParams{
				EventID:          row.ID,
				FeedbackID:       h.ID,
				PublishedVersion: d.Version,
			},
		)
		if err != nil {
			return out, wrap("read feedback acknowledgements", err)
		}
		for _, r := range rows {
			out.Acknowledgements = append(
				out.Acknowledgements,
				Acknowledgement{
					Version:  r.PublishedVersion,
					PersonID: r.PersonID,
					At:       r.AcknowledgedAt,
				},
			)
		}
	}
	if out.Draft == nil && out.Published == nil {
		return out, access.ErrNotFound
	}
	if a.Organiser || (a.Helper && h.Category != "competition_output") {
		rows, err := q.FeedbackEscalations(
			ctx,
			dbgen.FeedbackEscalationsParams{EventID: row.ID, FeedbackID: h.ID},
		)
		if err != nil {
			return out, wrap("read scoped feedback escalation", err)
		}
		for _, r := range rows {
			if r.RecipientID == actor || a.Organiser {
				out.Escalations = append(
					out.Escalations,
					Escalation{
						ID:               r.ID,
						ActionID:         r.ActionID,
						Cycle:            r.Cycle,
						SettingsVersion:  r.SettingsVersion,
						RecipientID:      r.RecipientID,
						ThresholdSeconds: r.ThresholdSeconds,
						At:               r.OccurredAt,
					},
				)
			}
		}
	}
	return out, nil
}

func (s Service) Read(ctx context.Context, actor, eventID, id uuid.UUID) (Page, error) {
	var out Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, eventID)
		if err != nil {
			return err
		}
		h, err := head(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		out, err = s.project(ctx, tx, q, row, actor, h)
		return err
	})
	return out, wrap("read shared feedback", err)
}

func (s Service) List(
	ctx context.Context,
	actor, eventID, teamID, after uuid.UUID,
	limit int32,
) (List, error) {
	out := List{Items: []Page{}}
	if _, err := pageLimit(limit); err != nil {
		return out, err
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, eventID)
		if err != nil {
			return err
		}
		if _, err = s.authority(ctx, q, actor, eventID, teamID, "setup"); err != nil {
			return err
		}
		cursor := after
		visibleIDs := []uuid.UUID{}
		for {
			rows, err := q.FeedbackHeadsPage(
				ctx,
				dbgen.FeedbackHeadsPageParams{
					EventID:  eventID,
					TeamID:   &teamID,
					AfterID:  cursor,
					PageSize: 100,
				},
			)
			if err != nil {
				return wrap("page feedback candidates", err)
			}
			for _, h := range rows {
				cursor = h.ID
				p, err := s.project(ctx, tx, q, row, actor, h)
				if errors.Is(err, access.ErrNotFound) || errors.Is(err, access.ErrForbidden) {
					continue
				}
				if err != nil {
					return err
				}
				if len(out.Items) == int(limit) {
					last := visibleIDs[len(visibleIDs)-1]
					out.NextCursor = &last
					return nil
				}
				out.Items = append(out.Items, p)
				visibleIDs = append(visibleIDs, h.ID)
			}
			if len(rows) < 100 {
				return nil
			}
		}
	})
	return out, wrap("list own team feedback", err)
}

func (s Service) History(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	before int64,
	limit int32,
) (History, error) {
	out := History{Items: []Document{}}
	if _, err := pageLimit(limit); err != nil {
		return out, err
	}
	if before == 0 {
		before = math.MaxInt64
	}
	if before < 1 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, eventID)
		if err != nil {
			return err
		}
		h, err := head(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		p, err := s.project(ctx, tx, q, row, actor, h)
		if err != nil {
			return err
		}
		for {
			rows, err := q.FeedbackHistoryPage(
				ctx,
				dbgen.FeedbackHistoryPageParams{
					EventID:       eventID,
					ID:            id,
					BeforeVersion: before,
					PageSize:      100,
				},
			)
			if err != nil {
				return wrap("read feedback history", err)
			}
			for _, raw := range rows {
				d, err := decodeDocument(raw)
				if err != nil {
					return err
				}
				before = d.Version
				if d.State != "published" {
					if !p.CanManage {
						continue
					}
					if sourceErr := s.sourceAuthority(
						ctx,
						tx,
						actor,
						eventID,
						d,
					); sourceErr != nil {
						if errors.Is(sourceErr, access.ErrForbidden) ||
							errors.Is(sourceErr, access.ErrNotFound) ||
							errors.Is(sourceErr, access.ErrConflict) {
							continue
						}
						return sourceErr
					}
				}
				if len(out.Items) == int(limit) {
					v := out.Items[len(out.Items)-1].Version
					out.NextCursor = &v
					return nil
				}
				out.Items = append(
					out.Items,
					s.projectDocument(ctx, tx, actor, eventID, d, p.CanManage),
				)
			}
			if len(rows) < 100 {
				return nil
			}
		}
	})
	return out, wrap("read shared feedback history", err)
}

func (s Service) Acknowledge(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	version int64,
	commandID uuid.UUID,
) (Page, error) {
	var out Page
	hash, err := digest(struct {
		ID      uuid.UUID `json:"id"`
		Version int64     `json:"version"`
	}{id, version})
	if err != nil {
		return out, err
	}
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, eventID)
		if err != nil {
			return err
		}
		if !row.WritesAllowed {
			return access.ErrForbidden
		}
		h, err := head(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		p, err := s.project(ctx, tx, q, row, actor, h)
		if err != nil {
			return err
		}
		if !p.CanAcknowledge || p.Published == nil {
			return access.ErrForbidden
		}
		v, replay, err := duplicate(ctx, q, eventID, actor, commandID, id, "ack", hash)
		if err != nil {
			return err
		}
		if replay {
			out = p
			out.CommandVersion = &v
			return nil
		}
		if version != p.Published.Version {
			return access.ErrConflict
		}
		if err = q.AcknowledgeFeedback(
			ctx,
			dbgen.AcknowledgeFeedbackParams{
				EventID:          eventID,
				FeedbackID:       id,
				PublishedVersion: version,
				PersonID:         actor,
				AcknowledgedAt:   s.now(),
			},
		); err != nil {
			return wrap("acknowledge exact feedback release", err)
		}
		if err = command(ctx, q, eventID, actor, commandID, id, "ack", hash, version); err != nil {
			return err
		}
		if err = record(ctx, q, eventID, actor, id, version); err != nil {
			return err
		}
		out, err = s.project(ctx, tx, q, row, actor, h)
		out.CommandVersion = &version
		return err
	})
	return out, wrap("acknowledge shared feedback", err)
}

func (s Service) Candidates(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
) ([]Candidate, error) {
	out := []Candidate{}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, eventID)
		if err != nil {
			return err
		}
		h, err := head(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		p, err := s.project(ctx, tx, q, row, actor, h)
		if err != nil {
			return err
		}
		if !p.CanManage || !row.WritesAllowed {
			return access.ErrForbidden
		}
		people, err := q.HelpRecipientCandidates(
			ctx,
			dbgen.HelpRecipientCandidatesParams{EventID: eventID, TeamID: h.TeamID},
		)
		if err != nil {
			return wrap("current feedback owner candidates", err)
		}
		for _, id := range people {
			a, err := s.authority(ctx, q, id, eventID, h.TeamID, h.Category)
			if errors.Is(err, access.ErrNotFound) || errors.Is(err, access.ErrForbidden) {
				continue
			}
			if err != nil {
				return err
			}
			person, err := q.Person(ctx, id)
			if err != nil {
				return wrap("owner candidate name", err)
			}
			out = append(
				out,
				Candidate{PersonID: id, Name: person.DisplayName, TeamMember: a.TeamMember},
			)
		}
		return nil
	})
	return out, wrap("derive feedback owner candidates", err)
}
