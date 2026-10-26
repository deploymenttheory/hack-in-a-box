package loadprofile

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

func TestKnownReleasePrefixRejectsUnknownControlOrAnyDomainProgress(t *testing.T) {
	plan, err := NewReleaseFactoryPlan(
		uuid.New(),
		time.Date(2026, time.October, 26, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	source := SourcePreparation{OriginalAdmissionSHA256: strings.Repeat("a", 64)}
	for _, planned := range plan.Events {
		owner := uuid.New()
		source.Sources = append(
			source.Sources,
			PreparedEventSources{
				Event:     event.View{ID: planned.Input.ID, OwnerID: &owner},
				Challenge: content.Page{ID: uuid.New()},
			},
		)
	}
	sourceSHA := strings.Repeat("b", 64)
	clock := &DockerSetupClock{Project: "hackbox-load123-prefix-control"}
	makePrefix := func() ReleasePreparation {
		publication := plan.Events[4].Input.Configuration.LiveStart.AddDate(0, 0, -7)
		prefix := ReleasePreparation{
			OriginalSourcesSHA256:   sourceSHA,
			OriginalAdmissionSHA256: source.OriginalAdmissionSHA256,
			Step:                    "archive-native-T-7-release",
			ClockSteps: []SetupClockStep{
				{
					Expected:         plan.SetupStart,
					Next:             publication,
					OriginalComplete: true,
					Elapsed:          time.Second,
					Container: fmt.Sprintf(
						"%s-clock-%d",
						clock.Project,
						publication.UnixNano(),
					),
					OutputSHA256: strings.Repeat("c", 64),
				},
			},
		}
		prefix.Authentication = AuthenticationReceipt{Authenticated: 1000, DistinctPeople: 1000}
		prefix.SessionCleanup = SetupSessionCleanup{OriginalACKs: 1000}
		prefix.AfterCleanup = &InstallationCensus{Events: 5, People: 1000, ActivePeople: 1000}
		for _, item := range source.Sources {
			prefix.Calls = append(
				prefix.Calls,
				PreparationCall{
					Actor:            *item.Event.OwnerID,
					Method:           http.MethodGet,
					Path:             "/api/v1/events/" + item.Event.ID.String(),
					Status:           200,
					OriginalComplete: true,
				},
			)
		}
		archive := source.Sources[4]
		prefix.Calls = append(
			prefix.Calls,
			PreparationCall{
				Actor:            *archive.Event.OwnerID,
				Method:           http.MethodGet,
				Path:             "/api/v1/events/" + archive.Event.ID.String() + "/content/" + archive.Challenge.ID.String(),
				Status:           404,
				OriginalComplete: true,
			},
		)
		for range 1000 {
			prefix.Calls = append(
				prefix.Calls,
				PreparationCall{
					Actor:            uuid.New(),
					Method:           http.MethodPost,
					Path:             "/api/v1/logout",
					Status:           204,
					OriginalComplete: true,
				},
			)
		}
		return prefix
	}
	check := func(prefix ReleasePreparation) error {
		raw, err := json.Marshal(prefix)
		if err != nil {
			t.Fatal(err)
		}
		_, known, err := parseKnownArchivePrefix(raw, plan, source, sourceSHA, clock)
		if err == nil && !known {
			t.Fatal("supplied checkpoint silently ignored")
		}
		return err
	}
	if err := check(makePrefix()); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ReleasePreparation){
		"unknown-clock-response":   func(p *ReleasePreparation) { p.ClockSteps[0].OriginalComplete = false },
		"changed-source-hash":      func(p *ReleasePreparation) { p.OriginalSourcesSHA256 = strings.Repeat("d", 64) },
		"another-clock-step":       func(p *ReleasePreparation) { p.ClockSteps = append(p.ClockSteps, p.ClockSteps[0]) },
		"different-phase":          func(p *ReleasePreparation) { p.Step = "archive-native-live-and-formal-URL" },
		"domain-post":              func(p *ReleasePreparation) { p.Calls[0].Method = http.MethodPost },
		"unrecognised-get":         func(p *ReleasePreparation) { p.Calls[0].Path += "/judging/brief" },
		"unknown-original-request": func(p *ReleasePreparation) { p.Calls[0].OriginalComplete = false },
		"formal-progress":          func(p *ReleasePreparation) { p.Formal = []submissions.Page{{TeamID: uuid.New()}} },
		"old-session":              func(p *ReleasePreparation) { p.AfterCleanup.UnexpiredSessions = 1 },
		"unknown-cleanup":          func(p *ReleasePreparation) { p.SessionCleanup.Unresolved = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			prefix := makePrefix()
			mutate(&prefix)
			if err := check(prefix); err == nil {
				t.Fatal("unrecognised partial checkpoint accepted")
			}
		})
	}
	if _, known, err := parseKnownArchivePrefix(
		nil,
		plan,
		source,
		sourceSHA,
		clock,
	); err != nil ||
		known {
		t.Fatal("fresh path requires no inferred prefix", err)
	}
}
