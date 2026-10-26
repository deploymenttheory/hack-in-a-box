package loadprofile

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
)

var errCohort = errors.New("invalid native load cohort")

// CohortMember describes fixture identity selection, not an authority grant.
// Event roles and registrations must already exist through native preparation;
// runtime requests still pass the application's current authorization checks.
type CohortMember struct {
	Subject string    `json:"subject"`
	Event   uuid.UUID `json:"event"`
	Staff   bool      `json:"staff"`
	Surface Surface   `json:"surface"`
}

type CohortPlan struct {
	Profile string         `json:"profile"`
	Members []CohortMember `json:"members"`
}

func (p CohortPlan) Validate() error {
	if p.Profile != "release" && p.Profile != "smoke" {
		return fmt.Errorf("%w: explicit release or non-release smoke profile required", errCohort)
	}
	if len(p.Members) == 0 || len(p.Members) > 1000 {
		return fmt.Errorf("%w: bounded nonempty distinct people required", errCohort)
	}
	type census struct{ people, staff int }
	events := make(map[uuid.UUID]census)
	subjects := make(map[string]bool, len(p.Members))
	for _, member := range p.Members {
		if member.Subject == "" || len(member.Subject) > 128 || subjects[member.Subject] ||
			member.Event == uuid.Nil || (member.Surface != PublicSurface && member.Surface != AdminSurface) {
			return fmt.Errorf(
				"%w: unique subject, native event and explicit portal required",
				errCohort,
			)
		}
		subjects[member.Subject] = true
		c := events[member.Event]
		c.people++
		if member.Staff {
			c.staff++
		}
		events[member.Event] = c
	}
	if len(events) > 5 {
		return fmt.Errorf(
			"%w: extra event cannot be concealed in the five-event profile",
			errCohort,
		)
	}
	if p.Profile == "release" {
		if len(p.Members) != 1000 || len(events) != 5 {
			return fmt.Errorf(
				"%w: release requires five events and 1000 distinct people",
				errCohort,
			)
		}
		for _, c := range events {
			if c.people != 200 || c.staff != 20 {
				return fmt.Errorf(
					"%w: each event requires 180 competitors plus 20 staff",
					errCohort,
				)
			}
		}
	}
	return nil
}

type CohortActor struct {
	Actor
	Event uuid.UUID
	Staff bool
}

// AuthenticationReceipt is setup evidence only. Even 1000 successful native
// sign-ins establish no sustained activity; RunStage measures that separately.
// Subjects, native identities, jars and provider codes are deliberately absent.
type AuthenticationReceipt struct {
	Started        time.Time     `json:"started"`
	Elapsed        time.Duration `json:"monotonic_elapsed_ns"`
	Attempted      int           `json:"attempted"`
	Authenticated  int           `json:"authenticated"`
	DistinctPeople int           `json:"distinct_people"`
	Failed         int           `json:"failed"`
	Concurrency    int           `json:"concurrency"`
	SustainedProof bool          `json:"sustained_proof"`
	Unattempted    int           `json:"unattempted"`
}

// AuthenticateCohort signs in each declared person exactly once using normal
// OIDC/state/PKCE/callback/session HTTP. Concurrency is capped independently of
// the runtime load. There is no seeded session, retry or alternate auth route.
// The caller must close returned actors even when a partial setup returns error.
func AuthenticateCohort(
	ctx context.Context,
	plan CohortPlan,
	origins map[Surface]string,
	issuer string,
	transport http.RoundTripper,
	concurrency int,
) ([]CohortActor, AuthenticationReceipt, error) {
	plan.Members = slices.Clone(plan.Members)
	origins = maps.Clone(origins)
	receipt := AuthenticationReceipt{Concurrency: concurrency}
	if err := plan.Validate(); err != nil {
		return nil, receipt, err
	}
	if concurrency < 1 || concurrency > 20 {
		return nil, receipt, fmt.Errorf(
			"%w: login concurrency must be between one and twenty",
			errCohort,
		)
	}
	for _, member := range plan.Members {
		if err := validateSignInOrigins(
			member.Surface,
			origins[member.Surface],
			issuer,
		); err != nil {
			return nil, receipt, err
		}
	}
	receipt.Started = time.Now()
	actors := make([]CohortActor, len(plan.Members))
	var mu sync.Mutex
	var workers sync.WaitGroup
	queue := make(chan int)
	for range concurrency {
		workers.Go(func() {
			for index := range queue {
				member := plan.Members[index]
				session, err := SignIn(
					ctx,
					member.Surface,
					origins[member.Surface],
					issuer,
					member.Subject,
					transport,
				)
				mu.Lock()
				receipt.Attempted++
				if err != nil {
					receipt.Failed++
				} else {
					receipt.Authenticated++
					actors[index] = CohortActor{
						Actor: Actor{ID: member.Subject, Session: session},
						Event: member.Event,
						Staff: member.Staff,
					}
				}
				mu.Unlock()
			}
		})
	}
send:
	for index := range plan.Members {
		select {
		case queue <- index:
		case <-ctx.Done():
			break send
		}
	}
	close(queue)
	workers.Wait()
	receipt.Elapsed = time.Since(receipt.Started)
	receipt.Unattempted = len(plan.Members) - receipt.Attempted
	people := make(map[uuid.UUID]bool, len(actors))
	completed := make([]CohortActor, 0, receipt.Authenticated)
	for _, actor := range actors {
		if actor.Session != nil {
			people[actor.Session.Person()] = true
			completed = append(completed, actor)
		}
	}
	receipt.DistinctPeople = len(people)
	if receipt.Failed != 0 || receipt.Unattempted != 0 ||
		receipt.DistinctPeople != len(plan.Members) {
		return completed, receipt, fmt.Errorf(
			"%w: native authentication incomplete or identities collapsed",
			errCohort,
		)
	}
	return completed, receipt, nil
}
