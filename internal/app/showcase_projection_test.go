package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
)

func TestShowcaseNativePublicationFiltersAwardsAndCurrentAudience(t *testing.T) {
	s := newShowcaseFixture(t, false)
	f, r := s.results, s.results.runtime
	ctx := t.Context()
	project, err := f.public.deps.Outcomes.Public(ctx, r.competitor.ID, s.winnerProject)
	resultsSourceRequire(t, err)
	if len(project.Awards) != 0 || len(project.Links) != 2 {
		t.Fatal("private confirmation must not appear; reviewed current media should", project)
	}
	page, err := f.public.deps.Outcomes.FindProjects(
		ctx,
		r.competitor.ID,
		outcomes.ProjectQuery{
			Limit:   1,
			EventID: r.eventID,
			Status:  "proposed",
			Search:  "NON-WINNING",
		},
	)
	resultsSourceRequire(t, err)
	if len(page.Items) != 1 || page.Items[0].ID != s.nonWinnerProject || len(page.Events) != 2 {
		t.Fatal("native reviewed event/status/search filters", page)
	}
	seen := map[uuid.UUID]bool{}
	cursor := uuid.Nil
	for {
		page, err = f.public.deps.Outcomes.FindProjects(
			ctx,
			r.competitor.ID,
			outcomes.ProjectQuery{Limit: 1, Cursor: cursor},
		)
		resultsSourceRequire(t, err)
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatal("duplicate cursor item")
			}
			seen[item.ID] = true
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if len(seen) != 4 || seen[s.draftProject] {
		t.Fatal("permission-filtered native pagination", seen)
	}
	page, err = f.public.deps.Outcomes.FindProjects(
		ctx,
		r.competitor.ID,
		outcomes.ProjectQuery{Limit: 25, Search: "PRIVATE-"},
	)
	resultsSourceRequire(t, err)
	if len(page.Items) != 0 {
		t.Fatal("restricted source prose searchable")
	}
	if _, err = f.public.deps.Outcomes.Public(
		ctx,
		r.competitor.ID,
		s.draftProject,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("guessed draft", err)
	}
	if _, err = f.public.deps.Outcomes.FindProjects(
		ctx,
		r.competitor.ID,
		outcomes.ProjectQuery{Limit: 25, Status: "invented"},
	); !errors.Is(
		err,
		access.ErrInvalid,
	) {
		t.Fatal("invalid status", err)
	}
	resultsSourceRequire(t, s.announce(ctx))
	project, err = f.public.deps.Outcomes.Public(ctx, r.competitor.ID, s.winnerProject)
	resultsSourceRequire(t, err)
	if len(project.Awards) != 1 || project.Awards[0].Name != "Reviewed recovery award" {
		t.Fatal("explicit native award missing", project)
	}
	outsider, err := f.public.deps.Outcomes.Public(ctx, s.outsider.ID, s.winnerProject)
	resultsSourceRequire(t, err)
	if len(outsider.Awards) != 0 || len(outsider.Links) != 1 || outsider.SourceReadable {
		t.Fatal("project sharing broadened result/media/source authority", outsider)
	}
	nonWinner, err := f.public.deps.Outcomes.Public(ctx, r.competitor.ID, s.nonWinnerProject)
	resultsSourceRequire(t, err)
	if len(nonWinner.Awards) != 0 {
		t.Fatal("other team's award copied", nonWinner)
	}
	request := qualificationHTTPRequest(t)
	for _, surface := range []rehearsalSurface{f.admin, f.public} {
		status, raw := request(
			surface,
			r.competitor,
			"GET",
			fmt.Sprintf("/api/v1/projects?event_id=%s&status=accepted&limit=1", r.eventID),
			nil,
			0,
			true,
		)
		if status != 200 {
			t.Fatalf("canonical showcase HTTP: %d %s", status, raw)
		}
		_ = resultAPIDecode[contract.OutcomeProjectPage](t, "OutcomeProjectPage", raw)
	}
	resultsSourceRequire(t, s.correct(ctx))
	project, err = f.public.deps.Outcomes.Public(ctx, r.competitor.ID, s.winnerProject)
	resultsSourceRequire(t, err)
	if project.Version != 4 || project.Solution != "Corrected approved recovery approach." ||
		project.FollowUpStatus != "in_progress" {
		t.Fatal("deliberate corrected version", project)
	}
	resultsSourceRequire(t, s.withdraw(ctx, s.nonWinnerProject))
	if _, err = f.public.deps.Outcomes.Public(
		ctx,
		r.competitor.ID,
		s.nonWinnerProject,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("withdrawn project route", err)
	}
	var versions int
	resultsSourceRequire(
		t,
		r.pool.QueryRow(ctx, "SELECT count(*) FROM outcome_project_versions WHERE outcome_id=$1", s.nonWinnerProject).
			Scan(&versions),
	)
	if versions != 3 {
		t.Fatal("withdrawal erased reviewed history", versions)
	}
	resultsSourceRequire(t, s.archive(ctx))
	project, err = f.public.deps.Outcomes.Public(ctx, r.competitor.ID, s.winnerProject)
	resultsSourceRequire(t, err)
	if !project.Archived || project.Version != 4 || len(project.Awards) != 1 {
		t.Fatal("actual archive changed approved projection", project)
	}
	consent := privacy.Service{Pool: r.pool, Clock: r.clock.read}
	current, err := consent.OwnPhotoConsent(ctx, r.competitor.ID, r.eventID)
	resultsSourceRequire(t, err)
	_, err = consent.SetOwnPhotoConsent(ctx, r.competitor.ID, r.eventID, current.Revision, "no")
	resultsSourceRequire(t, err)
	project, err = f.public.deps.Outcomes.Public(ctx, r.competitor.ID, s.winnerProject)
	resultsSourceRequire(t, err)
	if len(project.Links) != 1 {
		t.Fatal("revoked media reference retained", project.Links)
	}
	_, err = r.pool.Exec(
		ctx,
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		r.eventID,
		r.competitor.ID,
	)
	resultsSourceRequire(t, err)
	if _, err = f.public.deps.Outcomes.Public(
		ctx,
		r.competitor.ID,
		s.restrictedProject,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("current event audience lost", err)
	}
	project, err = f.public.deps.Outcomes.Public(ctx, r.competitor.ID, s.winnerProject)
	resultsSourceRequire(t, err)
	if len(project.Awards) != 0 || project.SourceReadable {
		t.Fatal("org project retained narrower award/source access", project)
	}
	raw, err := json.Marshal(project)
	resultsSourceRequire(t, err)
	for _, secret := range []string{"PRIVATE-", resultsMentorPrivateNote, "source_binding", "votes", "confirmation", "judge_id", "recording_url", "owner_id"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("restricted field in showcase", secret)
		}
	}
	// The trusted completion timestamp must preserve the exact existing 90-day
	// boundary; the archive/read fix never widens organisation retention.
	completed, readErr := dbgen.New(r.pool).Event(ctx, r.eventID)
	resultsSourceRequire(t, readErr)
	if !completed.CompletedAt.Valid {
		t.Fatal("native completion retention origin missing")
	}
	boundary := completed.CompletedAt.Time.AddDate(0, 0, 90)
	r.clock.set(boundary.Add(-time.Nanosecond))
	if _, err = f.public.deps.Outcomes.Public(ctx, r.competitor.ID, s.winnerProject); err != nil {
		t.Fatal("retained project disappeared before exact boundary", err)
	}
	r.clock.set(boundary)
	if _, err = f.public.deps.Outcomes.Public(
		ctx,
		r.competitor.ID,
		s.winnerProject,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("expired project survived exact retention boundary", err)
	}
}
