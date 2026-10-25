package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/media"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type showcaseFixture struct {
	results                                                                         *resultsSourceFixture
	nonWinner, secondEvent, secondTeam, prize, mediaID                              uuid.UUID
	winnerProject, nonWinnerProject, secondProject, draftProject, restrictedProject uuid.UUID
	outsider                                                                        dbgen.Person
	fields                                                                          map[uuid.UUID]outcomes.PublicationFields
}

// Only identities/membership are synthetic facts. Competition evidence, judging,
// awards, media consent/release and every project version use actual native owners.
func newShowcaseFixture(t *testing.T, paginate bool) *showcaseFixture {
	t.Helper()
	s := &showcaseFixture{prize: uuid.New(), fields: map[uuid.UUID]outcomes.PublicationFields{}}
	s.results, s.nonWinner = newTwoTeamResultsFixture(
		t,
		[2]int{3, 4},
		func(d *rubric.Definition) { d.Prizes = []rubric.Prize{{ID: s.prize, Name: "Reviewed recovery award"}} },
	)
	f, r := s.results, s.results.runtime
	ctx := t.Context()
	s.outsider = testdb.Person(t, r.pool)
	_, err := r.pool.Exec(
		ctx,
		"UPDATE events SET name='Synthetic autumn hack' WHERE id=$1",
		r.eventID,
	)
	resultsSourceRequire(t, err)
	s.secondEvent = r.otherEventID
	s.secondTeam = uuid.New()
	config := event.DefaultConfiguration()
	config.MinimumAttendance = 1
	config.LiveStart = r.clock.read().Add(-2 * time.Hour)
	config.LiveEnd = r.clock.read().Add(2 * time.Hour)
	config.SubmissionCutoff = config.LiveEnd
	raw, err := json.Marshal(config)
	resultsSourceRequire(t, err)
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{"UPDATE events SET name='Synthetic spring hack',stage='live',configuration=$2,published_at=$3 WHERE id=$1", []any{s.secondEvent, raw, config.LiveStart.Add(-7 * 24 * time.Hour)}},
		{"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')", []any{s.secondEvent, r.competitor.ID}},
		{"INSERT INTO hack_teams(event_id,id,name,state,frozen,published_captain_id)VALUES($1,$2,'Synthetic spring team','published',true,$3)", []any{s.secondEvent, s.secondTeam, r.competitor.ID}},
		{"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'published')", []any{s.secondEvent, s.secondTeam, r.competitor.ID}},
	} {
		_, err = r.pool.Exec(ctx, statement.query, statement.args...)
		resultsSourceRequire(t, err)
	}
	consent := privacy.Service{Pool: r.pool, Clock: r.clock.read}
	_, err = consent.SetOwnPhotoConsent(ctx, r.competitor.ID, r.eventID, 0, "yes")
	resultsSourceRequire(t, err)
	revision := func() int64 {
		row, e := dbgen.New(r.pool).Event(ctx, r.eventID)
		resultsSourceRequire(t, e)
		return row.Revision
	}
	m := f.admin.deps.Media
	item, err := m.Change(
		ctx,
		r.owner.ID,
		r.eventID,
		uuid.New(),
		revision(),
		media.Command{
			Action:    "save",
			CommandID: uuid.New(),
			Reason:    "Review synthetic recovery demonstration",
			Metadata: &media.Metadata{
				Kind:         "recording",
				Title:        "Reviewed recovery demonstration",
				RecordingURL: "https://recordings.example.test/PRIVATE-NATIVE-RECORDING",
				People:       []uuid.UUID{r.competitor.ID},
			},
		},
	)
	resultsSourceRequire(t, err)
	item, err = m.Change(
		ctx,
		r.owner.ID,
		r.eventID,
		item.ID,
		revision(),
		media.Command{
			Action:    "review",
			Version:   item.Version,
			CommandID: uuid.New(),
			Reason:    "Review content and current consent",
			Review:    &media.Review{ContentReviewed: true, IdentificationReviewed: true},
		},
	)
	resultsSourceRequire(t, err)
	item, err = m.Change(
		ctx,
		r.owner.ID,
		r.eventID,
		item.ID,
		revision(),
		media.Command{
			Action:    "publish",
			Version:   item.Version,
			CommandID: uuid.New(),
			Reason:    "Deliberately release reviewed synthetic media",
		},
	)
	resultsSourceRequire(t, err)
	s.mediaID = item.ID
	s.winnerProject = s.publish(
		t,
		r.eventID,
		r.teamID,
		"Recover services with less effort",
		"organisation",
		"accepted",
		[]outcomes.MediaReference{{EventID: r.eventID, ID: item.ID, Version: item.Version}},
	)
	s.restrictedProject = s.publish(
		t,
		r.eventID,
		s.nonWinner,
		"Event audience project",
		"event",
		"accepted",
		nil,
	)
	s.nonWinnerProject = s.publish(
		t,
		r.eventID,
		s.nonWinner,
		"Useful work from a non-winning team",
		"organisation",
		"proposed",
		nil,
	)
	s.secondProject = s.publish(
		t,
		s.secondEvent,
		s.secondTeam,
		"Explore work across events",
		"organisation",
		"in_progress",
		nil,
	)
	// This actual native draft is never published; its private text must not enter
	// search, facets, guessed routes or media previews.
	s.draftProject = uuid.New()
	_, err = f.admin.deps.Outcomes.Change(
		ctx,
		r.owner.ID,
		s.draftProject,
		0,
		outcomes.Command{
			ID:     uuid.New(),
			Action: "propose",
			Reason: "Synthetic private unpublished proposal",
			Definition: &outcomes.Definition{
				Kind:          "team_outcome",
				Problem:       "PRIVATE-UNPUBLISHED-PROJECT",
				Audience:      "PRIVATE-PEOPLE",
				ProposedValue: "PRIVATE-ESTIMATE",
				Solution:      "PRIVATE-UNREVIEWED-SOLUTION",
			},
			Provenance: &outcomes.Provenance{EventID: &r.eventID, TeamID: &r.teamID},
		},
	)
	resultsSourceRequire(t, err)
	if paginate {
		for i := 0; i < 26; i++ {
			s.publish(
				t,
				s.secondEvent,
				s.secondTeam,
				fmt.Sprintf("Synthetic reviewed project %02d", i),
				"organisation",
				"accepted",
				nil,
			)
		}
	}
	p, err := f.service.Read(ctx, r.owner.ID, r.eventID)
	resultsSourceRequire(t, err)
	p, err = f.service.Change(
		ctx,
		r.owner.ID,
		r.eventID,
		p.Revision,
		results.Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			Action:        "prepare",
			SourceBinding: p.Preview.Binding,
			Reason:        "Prepare actual non-tied independent award source",
		},
	)
	resultsSourceRequire(t, err)
	_, err = f.service.Change(
		ctx,
		r.owner.ID,
		r.eventID,
		p.Revision,
		results.Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			Action:        "confirm",
			SourceBinding: p.Preview.Binding,
			Reason:        "Privately confirm award without publishing it",
			Awards: []results.Award{
				{PrizeID: s.prize, Name: "Reviewed recovery award", TeamID: r.teamID},
			},
		},
	)
	resultsSourceRequire(t, err)
	return s
}

func (s *showcaseFixture) publish(
	t *testing.T,
	eventID, teamID uuid.UUID,
	problem, audience, status string,
	refs []outcomes.MediaReference,
) uuid.UUID {
	t.Helper()
	f, r := s.results, s.results.runtime
	ctx := t.Context()
	id := uuid.New()
	value, err := f.admin.deps.Outcomes.Change(
		ctx,
		r.owner.ID,
		id,
		0,
		outcomes.Command{
			ID:     uuid.New(),
			Action: "propose",
			Reason: "Capture synthetic team output privately",
			Definition: &outcomes.Definition{
				Kind:          "team_outcome",
				Problem:       "PRIVATE-ORIGINAL-PROBLEM",
				Audience:      "PRIVATE-ORIGINAL-AUDIENCE",
				ProposedValue: "PRIVATE-UNVERIFIED-ESTIMATE",
				Solution:      "PRIVATE-ORIGINAL-SOLUTION",
			},
			Provenance: &outcomes.Provenance{EventID: &eventID, TeamID: &teamID},
		},
	)
	resultsSourceRequire(t, err)
	value, err = f.admin.deps.Outcomes.Change(
		ctx,
		r.owner.ID,
		id,
		value.CurrentVersion,
		outcomes.Command{
			ID:     uuid.New(),
			Action: "moderate",
			Status: status,
			Reason: "Review follow-up status separately from demonstrated adoption",
		},
	)
	resultsSourceRequire(t, err)
	fields := outcomes.PublicationFields{
		Problem:        problem,
		Solution:       "Approved recovery approach and next steps.",
		Audience:       audience,
		ShareEventName: true,
		ShareTeamName:  true,
		Links: []outcomes.ProjectLink{
			{
				ID:    uuid.New(),
				Label: "Approved private source repository",
				URL:   "https://github.com/synthetic-org/private-project",
			},
		},
		Media: refs,
	}
	publication, err := f.admin.deps.Outcomes.ChangePublication(
		ctx,
		r.owner.ID,
		id,
		0,
		outcomes.PublicationCommand{
			ID:            uuid.New(),
			Action:        "save",
			SourceVersion: value.CurrentVersion,
			Fields:        &fields,
			Reason:        "Review exactly these positive fields",
		},
	)
	resultsSourceRequire(t, err)
	_, err = f.admin.deps.Outcomes.ChangePublication(
		ctx,
		r.owner.ID,
		id,
		publication.Version,
		outcomes.PublicationCommand{
			ID:            uuid.New(),
			Action:        "publish",
			SourceVersion: value.CurrentVersion,
			Reason:        "Explicitly publish independently of awards",
		},
	)
	resultsSourceRequire(t, err)
	s.fields[id] = fields
	return id
}

func (s *showcaseFixture) announce(ctx context.Context) error {
	f, r := s.results, s.results.runtime
	p, err := f.service.Read(ctx, r.owner.ID, r.eventID)
	if err != nil {
		return err
	}
	_, err = f.service.Change(
		ctx,
		r.owner.ID,
		r.eventID,
		p.Revision,
		results.Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			Action:        "publish_awards",
			SourceBinding: p.Preview.Binding,
			Reason:        "Explicit announcement after private confirmation",
		},
	)
	return err
}

func (s *showcaseFixture) withdraw(ctx context.Context, id uuid.UUID) error {
	f, r := s.results, s.results.runtime
	p, err := f.admin.deps.Outcomes.Publication(ctx, r.owner.ID, id)
	if err != nil {
		return err
	}
	v, err := f.admin.deps.Outcomes.Read(ctx, r.owner.ID, id, 0)
	if err != nil {
		return err
	}
	_, err = f.admin.deps.Outcomes.ChangePublication(
		ctx,
		r.owner.ID,
		id,
		p.Version,
		outcomes.PublicationCommand{
			ID:            uuid.New(),
			Action:        "withdraw",
			SourceVersion: v.CurrentVersion,
			Reason:        "Explicitly withdraw future discovery; retain safe history",
		},
	)
	return err
}

func (s *showcaseFixture) correct(ctx context.Context) error {
	f, r := s.results, s.results.runtime
	v, err := f.admin.deps.Outcomes.Read(ctx, r.owner.ID, s.winnerProject, 0)
	if err != nil {
		return err
	}
	v, err = f.admin.deps.Outcomes.Change(
		ctx,
		r.owner.ID,
		s.winnerProject,
		v.CurrentVersion,
		outcomes.Command{
			ID:     uuid.New(),
			Action: "moderate",
			Status: "in_progress",
			Reason: "Separately review current follow-up, never silently republish",
		},
	)
	if err != nil {
		return err
	}
	fields := s.fields[s.winnerProject]
	fields.Solution = "Corrected approved recovery approach."
	p, err := f.admin.deps.Outcomes.Publication(ctx, r.owner.ID, s.winnerProject)
	if err != nil {
		return err
	}
	p, err = f.admin.deps.Outcomes.ChangePublication(
		ctx,
		r.owner.ID,
		s.winnerProject,
		p.Version,
		outcomes.PublicationCommand{
			ID:            uuid.New(),
			Action:        "save",
			SourceVersion: v.CurrentVersion,
			Fields:        &fields,
			Reason:        "Review corrected positive projection",
		},
	)
	if err != nil {
		return err
	}
	_, err = f.admin.deps.Outcomes.ChangePublication(
		ctx,
		r.owner.ID,
		s.winnerProject,
		p.Version,
		outcomes.PublicationCommand{
			ID:            uuid.New(),
			Action:        "publish",
			SourceVersion: v.CurrentVersion,
			Reason:        "Explicitly release corrected projection",
		},
	)
	return err
}

func (s *showcaseFixture) archive(ctx context.Context) error {
	f, r := s.results, s.results.runtime
	for _, stage := range []string{"assessment", "completed", "archived"} {
		current, err := f.admin.deps.Events.Read(ctx, r.owner.ID, r.eventID)
		if err != nil {
			return err
		}
		if _, err = f.admin.deps.Events.Transition(
			ctx,
			r.owner.ID,
			r.eventID,
			current.Revision,
			stage,
			"Actual native archive without changing showcase publication",
			false,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *showcaseFixture) declineConsent(ctx context.Context) error {
	r := s.results.runtime
	p := privacy.Service{Pool: r.pool, Clock: r.clock.read}
	current, err := p.OwnPhotoConsent(ctx, r.competitor.ID, r.eventID)
	if err != nil {
		return err
	}
	_, err = p.SetOwnPhotoConsent(ctx, r.competitor.ID, r.eventID, current.Revision, "no")
	return err
}
