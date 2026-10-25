package app

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestResultsFinalistPublicationIsIndependentAndExactlyAcknowledged(t *testing.T) {
	f := newResultsSourceFixture(t, rubric.MentorQualification)
	ctx := t.Context()
	actor, eventID := f.runtime.owner.ID, f.runtime.eventID
	p, err := f.service.Read(ctx, actor, eventID)
	resultsSourceRequire(t, err)
	if p.Preview.Selection == nil || p.Preview.FinalistsBinding == "" || p.Preview.Binding != "" || len(p.Preview.Tallies) != 0 {
		t.Fatal("fixture pretended fresh judging was complete", p)
	}
	released, err := f.service.Released(ctx, f.runtime.competitor.ID, eventID)
	resultsSourceRequire(t, err)
	if len(released) != 0 {
		t.Fatal("preparation implicitly published", released)
	}
	prepare := results.Command{RequestID: uuid.New(), Version: p.Round.Version, Action: "prepare", SourceBinding: p.Preview.FinalistsBinding, Reason: "Review finalists while fresh final is pending"}
	p, err = f.service.Change(ctx, actor, eventID, p.Revision, prepare)
	resultsSourceRequire(t, err)
	if !p.Current || p.Receipt == nil || p.Round.Source.Selection == nil || len(p.Announcements) != 0 {
		t.Fatal("private qualification preparation lost phase/source", p)
	}
	current, err := f.service.Read(ctx, actor, eventID)
	resultsSourceRequire(t, err)
	if current.Receipt != nil {
		t.Fatal("current GET invented original acknowledgement")
	}
	command := results.Command{RequestID: uuid.New(), Version: current.Round.Version, Action: "publish_finalists", SourceBinding: current.Preview.FinalistsBinding, Reason: "Explicit finalist-only announcement"}
	expected := current.Revision
	accepted, err := f.service.Change(ctx, actor, eventID, expected, command)
	resultsSourceRequire(t, err)
	if accepted.Receipt == nil || accepted.Receipt.PublicationID == nil || len(accepted.Announcements) != 1 {
		t.Fatal("missing actual announcement ACK", accepted)
	}
	original := *accepted.Receipt
	announced := accepted.Announcements[0]
	if announced.Kind != "finalists" || len(announced.Finalists) != 1 || announced.Finalists[0].ID != f.runtime.teamID || announced.Finalists[0].Name == "" || len(announced.Awards) != 0 {
		t.Fatal("independent finalist release changed categories", announced)
	}
	raw, err := json.Marshal(announced)
	resultsSourceRequire(t, err)
	if strings.Contains(string(raw), resultsMentorPrivateNote) {
		t.Fatal("raw private mentor evidence published")
	}
	var fields map[string]json.RawMessage
	resultsSourceRequire(t, json.Unmarshal(raw, &fields))
	for _, key := range []string{"tallies", "votes", "evidence", "selection", "source_binding", "reason"} {
		if _, exists := fields[key]; exists {
			t.Fatal("private field in released DTO", key)
		}
	}
	repeated, err := f.service.Change(ctx, actor, eventID, expected, command)
	resultsSourceRequire(t, err)
	if repeated.Receipt == nil || !reflect.DeepEqual(*repeated.Receipt, original) || repeated.Round.Version != accepted.Round.Version || repeated.Revision != accepted.Revision {
		t.Fatal("exact retry duplicated or replaced original effect", repeated)
	}
	ack, err := f.service.OriginalReceipt(ctx, actor, eventID, command.RequestID)
	resultsSourceRequire(t, err)
	if ack.Receipt == nil || !reflect.DeepEqual(*ack.Receipt, original) {
		t.Fatal("original metadata changed", ack)
	}
	altered := command
	altered.Reason = "Different reviewed intent"
	if _, err = f.service.Change(ctx, actor, eventID, expected, altered); !errors.Is(err, access.ErrConflict) {
		t.Fatal("original key claimed another payload", err)
	}
	duplicate := command
	duplicate.RequestID = uuid.New()
	duplicate.Version = accepted.Round.Version
	if _, err = f.service.Change(ctx, actor, eventID, accepted.Revision, duplicate); !errors.Is(err, access.ErrConflict) {
		t.Fatal("same source published twice", err)
	}
	var publications, versions int
	resultsSourceRequire(t, f.runtime.pool.QueryRow(ctx, "SELECT count(*) FROM result_publications WHERE event_id=$1", eventID).Scan(&publications))
	resultsSourceRequire(t, f.runtime.pool.QueryRow(ctx, "SELECT count(*) FROM result_versions WHERE event_id=$1", eventID).Scan(&versions))
	if publications != 1 || versions != 2 {
		t.Fatal("failed duplicate mutated retained history", publications, versions)
	}
	released, err = f.service.Released(ctx, f.runtime.competitor.ID, eventID)
	resultsSourceRequire(t, err)
	if len(released) != 1 || released[0].ID != announced.ID {
		t.Fatal("announced source unavailable to current competitor", released)
	}
	_, err = f.runtime.pool.Exec(ctx, "UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2", eventID, f.runtime.competitor.ID)
	resultsSourceRequire(t, err)
	if _, err = f.service.Released(ctx, f.runtime.competitor.ID, eventID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("withdrawn competitor retained announcement access", err)
	}
	outsider := testdb.Person(t, f.runtime.pool)
	if _, err = f.service.Released(ctx, outsider.ID, eventID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("unapproved audience read announcements", err)
	}
	if _, err = f.service.Change(ctx, f.judges[0].ID, eventID, accepted.Revision, duplicate); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("judge published as organiser", err)
	}
}
func TestResultsPreparationRequiresAllIndependentJudgesAndOpenDeliberation(t *testing.T) {
	f := newResultsSourceFixture(t, rubric.AllTeams)
	ctx := t.Context()
	actor, eventID := f.runtime.owner.ID, f.runtime.eventID
	p, err := f.service.Read(ctx, actor, eventID)
	resultsSourceRequire(t, err)
	c := results.Command{RequestID: uuid.New(), Version: 0, Action: "prepare", Reason: "Cannot replace missing judges with zero"}
	if _, err = f.service.Change(ctx, actor, eventID, p.Revision, c); !errors.Is(err, access.ErrConflict) {
		t.Fatal("incomplete judges accepted", err)
	}
	f.complete(t)
	p, err = f.service.Read(ctx, actor, eventID)
	resultsSourceRequire(t, err)
	c.SourceBinding = p.Preview.Binding
	p, err = f.service.Change(ctx, actor, eventID, p.Revision, c)
	resultsSourceRequire(t, err)
	if !p.Current || p.Receipt == nil || len(p.Round.Source.Tallies) != 1 || p.Round.Source.Tallies[0].Total.Numerator != "42" {
		t.Fatal("actual full prepared tally incorrect", p)
	}
}
func TestConcurrentFinalistPublicationHasOneDurableSourceEvent(t *testing.T) {
	f := newResultsSourceFixture(t, rubric.MentorQualification)
	ctx := t.Context()
	actor, eventID := f.runtime.owner.ID, f.runtime.eventID
	p, err := f.service.Read(ctx, actor, eventID)
	resultsSourceRequire(t, err)
	command := results.Command{RequestID: uuid.New(), Version: 0, Action: "publish_finalists", SourceBinding: p.Preview.FinalistsBinding, Reason: "Concurrent deliberate finalist announcement"}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := f.service.Change(ctx, actor, eventID, p.Revision, command); errs <- err })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		resultsSourceRequire(t, err)
	}
	var count int
	resultsSourceRequire(t, f.runtime.pool.QueryRow(ctx, "SELECT count(*) FROM result_publications WHERE event_id=$1", eventID).Scan(&count))
	if count != 1 {
		t.Fatal("same captured concurrent request emitted duplicate event", count)
	}
}
