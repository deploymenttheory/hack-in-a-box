package certificates

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/resultpublication"
)

type attendanceFact struct {
	Day      string
	Present  bool
	Revision int64
}

func (s Service) eligibility(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.Event,
	target Target,
) (Eligibility, string, error) {
	out := Eligibility{Target: target, Blockers: []string{}}
	cfg, err := loadSettings(ctx, tx, row.ID)
	if err != nil {
		return out, "", failure("eligibility", err)
	}
	out.SettingsVersion = cfg.Version
	template := Template{}
	for _, t := range cfg.Templates {
		if t.ID == target.TemplateID {
			template = t
			break
		}
	}
	if template.ID == uuid.Nil {
		return out, "", access.ErrInvalid
	}
	out.Kind = template.Kind
	person, err := dbgen.New(tx).Person(ctx, target.PersonID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, "", access.ErrNotFound
	}
	if err != nil {
		return out, "", failure("eligibility", err)
	}
	if !person.Active {
		return out, "", access.ErrNotFound
	}
	var confirmed bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM competition_claims WHERE event_id=$1 AND person_id=$2 AND state='confirmed')", row.ID, target.PersonID).
		Scan(&confirmed)
	if err != nil {
		return out, "", failure("eligibility", err)
	}
	out.Fields = Fields{
		Name:        person.DisplayName,
		Event:       row.Name,
		Recognition: template.Recognition,
		Issuer:      template.Issuer,
		Accent:      template.Accent,
	}
	if !text(person.DisplayName, 160) {
		out.Blockers = append(out.Blockers, "recipient_name_required")
	}
	if !confirmed {
		return out, "", access.ErrNotFound
	}
	facts := eligibilityFacts{}
	key := "attendance"
	if template.Kind == "attendance" {
		if target.PublicationID != uuid.Nil || target.PrizeID != uuid.Nil {
			return out, "", access.ErrInvalid
		}
		facts.Attendance, err = s.attendanceEligibility(ctx, tx, row, cfg.Attendance, &out)
	} else {
		facts.Award, key, err = s.awardEligibility(ctx, tx, row, target, &out)
	}
	if err != nil {
		return out, "", err
	}

	out.Eligible = len(out.Blockers) == 0
	out.Binding, err = digest(struct {
		Eligibility   Eligibility
		EventRevision int64
		Facts         eligibilityFacts
	}{out, row.Revision, facts})
	return out, key, failure("eligibility", err)
}

func (s Service) Preview(
	ctx context.Context,
	actor, eventID uuid.UUID,
	target Target,
) (Eligibility, error) {
	var out Eligibility
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		row, staff, err := s.guard(ctx, tx, actor, eventID, false)
		if err != nil {
			return failure("eligibility", err)
		}
		if !staff {
			return access.ErrForbidden
		}
		out, _, err = s.eligibility(ctx, tx, row, target)
		return failure("eligibility", err)
	})
	return out, failure("eligibility", err)
}

type awardFact struct {
	Publication uuid.UUID
	Prize       uuid.UUID
	Team        uuid.UUID
	Version     int64
}
type eligibilityFacts struct {
	Attendance []attendanceFact
	Award      *awardFact
}

func (s Service) attendanceEligibility(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.Event,
	policy *AttendancePolicy,
	out *Eligibility,
) ([]attendanceFact, error) {
	if policy == nil {
		out.Blockers = append(out.Blockers, "attendance_policy_not_configured")
		return []attendanceFact{}, nil
	}
	var dates event.Configuration
	if err := json.Unmarshal(row.Configuration, &dates); err != nil {
		return nil, failure("attendance dates", err)
	}
	zone, err := time.LoadLocation(dates.Timezone)
	if err != nil {
		return nil, failure("attendance zone", err)
	}
	today := s.now().In(zone).Format(time.DateOnly)
	first, last := dates.LiveStart.In(zone).
		Format(time.DateOnly),
		dates.LiveEnd.Add(-time.Nanosecond).
			In(zone).
			Format(time.DateOnly)
	records := make([]attendanceFact, 0, len(policy.Days))
	present := 0
	for _, day := range policy.Days {
		f := attendanceFact{Day: day}
		err = tx.QueryRow(ctx, "SELECT present,revision FROM attendance_records WHERE event_id=$1 AND person_id=$2 AND attendance_day=$3::date", row.ID, out.Target.PersonID, day).
			Scan(&f.Present, &f.Revision)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, failure("recorded attendance", err)
		}
		records = append(records, f)
		if day >= first && day <= last && day <= today && f.Present {
			present++
		}
	}
	if present < policy.MinimumPresent {
		out.Blockers = append(out.Blockers, "recorded_attendance_below_policy")
	}
	return records, nil
}

func (s Service) awardEligibility(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.Event,
	target Target,
	out *Eligibility,
) (*awardFact, string, error) {
	if target.PublicationID == uuid.Nil || target.PrizeID == uuid.Nil {
		return nil, "", access.ErrInvalid
	}
	key := "award:" + target.PrizeID.String()
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT p.document FROM result_publications p WHERE p.event_id=$1 AND p.id=$2 AND p.kind='awards' AND p.published_at<=$3 AND NOT EXISTS(SELECT 1 FROM result_publications newer WHERE newer.event_id=p.event_id AND newer.kind='awards' AND newer.published_at<=$3 AND (newer.published_at,newer.id)>(p.published_at,p.id))", row.ID, target.PublicationID, s.now()).
		Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		out.Blockers = append(out.Blockers, "published_award_required")
		return nil, key, nil
	}
	if err != nil {
		return nil, "", failure("published award", err)
	}
	var publication resultpublication.Announcement
	if err = json.Unmarshal(raw, &publication); err != nil {
		return nil, "", failure("decode published award", err)
	}
	for _, award := range publication.Awards {
		if award.PrizeID != target.PrizeID {
			continue
		}
		var member bool
		err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hack_team_members m JOIN hack_teams t ON t.event_id=m.event_id AND t.id=m.team_id WHERE m.event_id=$1 AND m.person_id=$2 AND m.team_id=$3 AND m.snapshot='published' AND t.state='published' AND t.frozen)", row.ID, target.PersonID, award.Team.ID).
			Scan(&member)
		if err != nil {
			return nil, "", failure("award recipient", err)
		}
		if member {
			out.Fields.Award = award.Name
			return &awardFact{
				publication.ID,
				award.PrizeID,
				award.Team.ID,
				publication.Version,
			}, key, nil
		}
	}
	out.Blockers = append(out.Blockers, "published_award_recipient_required")
	return nil, key, nil
}
