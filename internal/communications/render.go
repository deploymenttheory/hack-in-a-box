package communications

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"html"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

// Render accepts plain text templates in every format. Both literal text and
// interpolated values are escaped, so stored markup never becomes active HTML.
func Render(source, format string, values map[string]string) (string, []string) {
	missing := []string{}
	result := placeholder.ReplaceAllStringFunc(source, func(token string) string {
		name := placeholder.FindStringSubmatch(token)[1]
		value, ok := values[name]
		if !ok || value == "" {
			missing = append(missing, name)
			return ""
		}
		return value
	})
	switch format {
	case "html":
		result = strings.ReplaceAll(html.EscapeString(result), "\n", "<br>\n")
	case "markdown":
		result = strings.NewReplacer("\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "(", "\\(", ")", "\\)", "#", "\\#", "!", "\\!", "<", "&lt;", ">", "&gt;", "&", "&amp;", "~", "\\~", "|", "\\|", "-", "\\-", "+", "\\+", ".", "\\.").
			Replace(result)
	}
	slices.Sort(missing)
	return result, slices.Compact(missing)
}

func selected(p dbgen.CommunicationPeopleRow, a Audience) bool {
	if p.CompetitionState == "confirmed" && slices.Contains(a.Roles, "attendee") {
		return true
	}
	for _, role := range p.Roles {
		if slices.Contains(a.Roles, role) {
			return true
		}
	}
	for _, state := range p.States {
		if slices.Contains(a.RegistrationStates, state) && (state != "offered" || p.OfferPending) {
			return true
		}
	}
	return p.CompetitionState == "confirmed" && slices.Contains(a.Teams, p.TeamID)
}

func factAllowed(
	ctx context.Context,
	q *dbgen.Queries,
	p dbgen.CommunicationPeopleRow,
	f dbgen.CommunicationFact,
) (bool, error) {
	if f.Status == "withdrawn" {
		return false, nil
	}
	if f.Kind == "help" || f.Kind == "outcome_review" {
		if f.OwnerID == nil || *f.OwnerID != p.ID {
			return false, nil
		}
	} else if f.Status != "published" {
		return false, nil
	}
	if f.ResourceID == nil || slices.Contains(p.Roles, "organiser") {
		return true, nil
	}
	allowed, err := q.ResourceAssigned(
		ctx,
		dbgen.ResourceAssignedParams{EventID: f.EventID, ResourceID: *f.ResourceID, PersonID: p.ID},
	)
	return allowed, contextError("factAllowed", err)
}

func outstanding(
	ctx context.Context,
	q *dbgen.Queries,
	p dbgen.CommunicationPeopleRow,
	a Audience,
	eventID uuid.UUID,
	facts []dbgen.CommunicationFact,
) (bool, error) {
	switch a.Outstanding {
	case "":
		return true, nil
	case "offer":
		return p.OfferPending, nil
	case "preparation":
		roles := append([]string{}, p.Roles...)
		if p.CompetitionState == "confirmed" {
			roles = append(roles, "attendee")
		}
		count, err := q.CommunicationOutstandingTasks(
			ctx,
			dbgen.CommunicationOutstandingTasksParams{
				EventID:  eventID,
				PersonID: p.ID.String(),
				Roles:    roles,
			},
		)
		return count > 0, contextError("outstanding", err)
	default:
		for _, f := range facts {
			if f.Kind == a.Outstanding &&
				(f.Status == "open" || ((f.Kind == "clarification" || f.Kind == "rubric") && f.Status == "published")) &&
				(a.FactID == nil || f.ID == *a.FactID) {
				ok, err := factAllowed(ctx, q, p, f)
				if err != nil || ok {
					return ok, contextError("outstanding", err)
				}
			}
		}
		return false, nil
	}
}

func eventValues(row dbgen.Event, c event.Configuration) map[string]string {
	values := map[string]string{
		"event.name":      row.Name,
		"event.workspace": "/events/" + row.ID.String(),
		"event.timezone":  c.Timezone,
	}
	for name, value := range map[string]string{"final_admission_cutoff": c.AdmissionCutoff.Format("2006-01-02T15:04:05Z07:00"), "team_freeze": c.TeamFreeze.Format("2006-01-02T15:04:05Z07:00"), "live_start": c.LiveStart.Format("2006-01-02T15:04:05Z07:00"), "live_end": c.LiveEnd.Format("2006-01-02T15:04:05Z07:00"), "submission_cutoff": c.SubmissionCutoff.Format("2006-01-02T15:04:05Z07:00")} {
		if !strings.HasPrefix(value, "0001-") {
			values["event."+name] = value
		}
	}
	if instant, err := schedule.ChallengePublication(
		c.LiveStart,
		c.Timezone,
		c.DSTChoice,
	); err == nil &&
		!c.LiveStart.IsZero() {
		values["event.challenge_publication"] = instant.Format("2006-01-02T15:04:05Z07:00")
	}
	return values
}

func resourceValues(
	ctx context.Context,
	q *dbgen.Queries,
	p dbgen.CommunicationPeopleRow,
	t Template,
	eventID uuid.UUID,
	values map[string]string,
	facts []dbgen.CommunicationFact,
) error {
	if p.CompetitionState == "confirmed" && p.TeamID != uuid.Nil {
		name, err := q.CommunicationTeam(
			ctx,
			dbgen.CommunicationTeamParams{EventID: eventID, ID: p.TeamID},
		)
		if err == nil {
			values["team.name"] = name
		}
	}
	if t.FactID == nil {
		return nil
	}
	for _, f := range facts {
		if f.ID == *t.FactID {
			ok, err := factAllowed(ctx, q, p, f)
			if err != nil {
				return contextError("resourceValues", err)
			}
			if ok {
				values[f.Kind+".revision"] = strconv.FormatInt(f.Revision, 10)
				values[f.Kind+".status"] = f.Status
			}
		}
	}
	return nil
}

func (s Service) contentValues(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	p dbgen.CommunicationPeopleRow,
	values map[string]string,
) error {
	items, err := q.ContentItems(
		ctx,
		dbgen.ContentItemsParams{EventID: row.ID, Cursor: uuid.Nil, PageSize: 1000},
	)
	if err != nil {
		return contextError("contentValues", err)
	}
	for _, item := range items {
		if item.Kind != "brief" && item.Kind != "challenge" {
			continue
		}
		if item.Kind == "challenge" {
			var c event.Configuration
			if err = json.Unmarshal(row.Configuration, &c); err != nil {
				return contextError("contentValues", err)
			}
			planned, planErr := schedule.ChallengePublication(c.LiveStart, c.Timezone, c.DSTChoice)
			if planErr != nil || s.now().Before(planned) || !row.PublishedAt.Valid ||
				s.now().Before(row.PublishedAt.Time) {
				continue
			}
		}
		version, err := access.ContentVersionForReader(ctx, q, p.ID, item, false)
		if err != nil {
			return contextError("contentValues", err)
		}
		if version == 0 || !item.ReleasedAt.Valid || s.now().Before(item.ReleasedAt.Time) {
			continue
		}
		snapshot, err := q.ContentVersion(
			ctx,
			dbgen.ContentVersionParams{EventID: row.ID, ContentID: item.ID, Version: version},
		)
		if err != nil {
			return contextError("contentValues", err)
		}
		var doc struct {
			Title string `json:"title"`
		}
		if err = json.Unmarshal(snapshot.Document, &doc); err != nil {
			return contextError("contentValues", err)
		}
		values[item.Kind+".title"] = doc.Title
	}
	return nil
}

func digest(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s Service) build(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	t Template,
	a Audience,
) (Preview, error) {
	if err := validateTemplate(t); err != nil {
		return Preview{}, contextError("build", err)
	}
	if err := validateAudience(a); err != nil {
		return Preview{}, contextError("build", err)
	}
	result := Preview{
		ID:              uuid.New(),
		TemplateID:      t.ID,
		TemplateVersion: t.Version,
		AudienceID:      a.ID,
		AudienceVersion: a.Version,
		Recipients:      []Recipient{},
		Blockers:        []string{},
	}
	var c event.Configuration
	if err := json.Unmarshal(row.Configuration, &c); err != nil {
		return result, contextError("build", err)
	}
	facts, err := q.CommunicationFacts(ctx, row.ID)
	if err != nil {
		return result, contextError("build", err)
	}
	people, err := q.CommunicationPeople(
		ctx,
		dbgen.CommunicationPeopleParams{EventID: row.ID, At: s.now()},
	)
	if err != nil {
		return result, contextError("build", err)
	}
	if len(people) > 10000 {
		return result, blocked("audience_too_large")
	}
	// Fingerprint approved non-personal facts. Audience membership is deliberately
	// re-evaluated at execution; changed content/date facts require fresh approval.
	items, err := q.ContentItems(
		ctx,
		dbgen.ContentItemsParams{EventID: row.ID, Cursor: uuid.Nil, PageSize: 1000},
	)
	if err != nil {
		return result, contextError("build", err)
	}
	for _, id := range []*uuid.UUID{t.FactID, a.FactID} {
		if id != nil &&
			!slices.ContainsFunc(
				facts,
				func(f dbgen.CommunicationFact) bool { return f.ID == *id },
			) {
			result.Blockers = append(result.Blockers, "unresolved_official_fact")
		}
	}
	approvedFacts := []dbgen.CommunicationFact{}
	if t.FactID != nil {
		for _, f := range facts {
			if f.ID == *t.FactID {
				approvedFacts = append(approvedFacts, f)
			}
		}
	}
	result.Fingerprint = digest(struct {
		Name          string
		Stage         string
		Configuration event.Configuration
		Facts         []dbgen.CommunicationFact
		Content       []dbgen.ContentItem
	}{row.Name, row.Stage, c, approvedFacts, items})
	for _, person := range people {
		if !selected(person, a) {
			continue
		}

		allowed, err := factRecipient(ctx, q, person, t.FactID, facts)
		if err != nil {
			return result, contextError("build fact audience", err)
		}
		if !allowed {
			continue
		}
		allowed, err = factRecipient(ctx, q, person, a.FactID, facts)
		if err != nil {
			return result, contextError("build selector fact", err)
		}
		if !allowed {
			continue
		}
		keep, err := outstanding(ctx, q, person, a, row.ID, facts)
		if err != nil {
			return result, contextError("build", err)
		}
		if !keep {
			continue
		}
		values := eventValues(row, c)
		values["person.name"] = person.DisplayName
		if err = resourceValues(ctx, q, person, t, row.ID, values, facts); err != nil {
			return result, contextError("build", err)
		}
		if err = s.contentValues(ctx, q, row, person, values); err != nil {
			return result, contextError("build", err)
		}
		subject, missing := Render(t.Subject, "text", values)
		body, missingBody := Render(t.Body, t.Format, values)
		missing = append(missing, missingBody...)
		slices.Sort(missing)
		missing = slices.Compact(missing)
		more, err := purposeMissing(ctx, q, row, t, values, c, facts, person)
		if err != nil {
			return result, contextError("build", err)
		}
		missing = append(missing, more...)
		result.Recipients = append(
			result.Recipients,
			Recipient{
				PersonID:   person.ID,
				Name:       person.DisplayName,
				Subject:    strings.NewReplacer("\r", " ", "\n", " ", "\x00", "").Replace(subject),
				Body:       body,
				Unresolved: missing,
			},
		)
	}
	if len(result.Recipients) == 0 {
		result.Blockers = append(result.Blockers, "empty_audience")
	}
	for _, r := range result.Recipients {
		if len(r.Unresolved) > 0 {
			result.Blockers = append(result.Blockers, "unresolved_placeholders")
			break
		}
	}
	return result, nil
}

func purposeMissing(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	t Template,
	values map[string]string,
	c event.Configuration,
	facts []dbgen.CommunicationFact,
	person dbgen.CommunicationPeopleRow,
) ([]string, error) {
	missing := []string{}
	if slices.Contains([]string{"help", "clarification", "rubric", "outcome_review"}, t.Purpose) &&
		values[t.Purpose+".revision"] == "" {
		missing = append(missing, t.Purpose+".revision")
	}
	for _, kind := range []string{"challenge", "brief"} {
		if t.Purpose == kind+"_release" && values[kind+".title"] == "" {
			missing = append(missing, kind+".title")
		}
	}
	if t.Purpose == "results" {
		published := false
		for _, f := range facts {
			if f.Kind == "results" && f.Status == "published" && t.FactID != nil &&
				f.ID == *t.FactID {
				allowed, err := factAllowed(ctx, q, person, f)
				published = allowed
				if err != nil {
					return nil, contextError("purposeMissing", err)
				}
			}
		}
		if !published {
			missing = append(missing, "results.published")
		}
	}
	if t.Purpose == "postponement" {
		notices, noticeErr := q.ContentNotices(
			ctx,
			dbgen.ContentNoticesParams{EventID: row.ID, After: 0},
		)
		if noticeErr != nil {
			return nil, contextError("purposeMissing", noticeErr)
		}
		approved := false
		for _, notice := range notices {
			if notice.LiveStart.Equal(c.LiveStart) && notice.LiveEnd.Equal(c.LiveEnd) &&
				notice.SubmissionCutoff.Equal(c.SubmissionCutoff) {
				approved = true
			}
		}
		if !approved {
			missing = append(missing, "postponement.approved")
		}
		used, _ := tokens(t.Subject + "\n" + t.Body)
		for _, required := range []string{"event.live_start", "event.live_end", "event.submission_cutoff"} {
			if !slices.Contains(used, required) {
				missing = append(missing, "postponement.required_dates")
				break
			}
		}
	}
	return missing, nil
}

func factRecipient(
	ctx context.Context,
	q *dbgen.Queries,
	person dbgen.CommunicationPeopleRow,
	id *uuid.UUID,
	facts []dbgen.CommunicationFact,
) (bool, error) {
	if id == nil {
		return true, nil
	}
	for _, f := range facts {
		if f.ID == *id {
			return factAllowed(ctx, q, person, f)
		}
	}
	// An unavailable source must remain unresolved; no placeholder is fabricated.
	return true, nil
}
