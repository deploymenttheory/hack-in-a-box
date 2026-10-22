package clarification_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func TestNotificationMetadataPagesFilterBeforeBoundsAndCurrentAudienceExcludesWithdrawal(
	t *testing.T,
) {
	f := setup(t)
	f.release()
	question, err := f.service.Ask(
		t.Context(),
		f.first,
		f.eventID,
		clarification.QuestionInput{
			TeamID:    f.team,
			CommandID: uuid.New(),
			Topic:     "challenge",
			Body:      "PRIVATE-QUESTION-PAGE-CANARY",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	source, _ := json.Marshal(question.Source)
	// Populate immutable publication tuples for the paging boundary. Actual
	// publication/approval and local delivery are exercised by automation's
	// separate native-workflow integration, not inferred from these fixtures.
	f.exec(
		`INSERT INTO clarification_questions(event_id,id,team_id,author_id,command_id,request_digest,topic,body,source,revision,status,created_at)SELECT $1,gen_random_uuid(),$2,$3,gen_random_uuid(),'synthetic','challenge','PRIVATE-QUESTION-PAGE-CANARY',$4,1,'answered',$5 FROM generate_series(1,1005)`,
		f.eventID,
		f.team,
		f.first,
		source,
		f.now,
	)
	f.exec(
		`INSERT INTO clarification_answers(event_id,id,question_id,revision,latest_version)SELECT event_id,gen_random_uuid(),id,1,1 FROM clarification_questions WHERE event_id=$1 AND status='answered'`,
		f.eventID,
	)
	f.exec(
		`INSERT INTO clarification_versions(event_id,answer_id,version,document,source,author_id,authored_at)SELECT event_id,id,1,'{"title":"Public answer","body":"Public clarification","kind":"clarification"}',$2,$3,$4 FROM clarification_answers WHERE event_id=$1`,
		f.eventID,
		source,
		f.owner,
		f.now,
	)
	f.exec(
		`INSERT INTO clarification_approvals(event_id,answer_id,version,actor_id,approved_at)SELECT event_id,id,1,$2,$3 FROM clarification_answers WHERE event_id=$1`,
		f.eventID,
		f.owner,
		f.now,
	)
	f.exec(
		`INSERT INTO clarification_publications(event_id,answer_id,version,actor_id,published_at)SELECT event_id,id,1,$2,$3 FROM clarification_answers WHERE event_id=$1`,
		f.eventID,
		f.owner,
		f.now,
	)
	f.exec(
		`UPDATE clarification_answers SET revision=2,approved_version=1,published_version=1 WHERE event_id=$1`,
		f.eventID,
	)
	var last uuid.UUID
	if err = f.pool.QueryRow(t.Context(), `SELECT id FROM clarification_answers WHERE event_id=$1 ORDER BY id DESC LIMIT 1`, f.eventID).
		Scan(&last); err != nil {
		t.Fatal(err)
	}
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		wanted, next, e := f.service.DiscoverNotificationSourcesPageInTransaction(
			t.Context(),
			tx,
			f.eventID,
			&last,
			uuid.Nil,
			1,
		)
		if e != nil {
			return e
		}
		if len(wanted) != 1 || wanted[0].AnswerID != last || next != nil {
			return fmt.Errorf("requested answer beyond first page missing: %+v", wanted)
		}
		seen := map[uuid.UUID]bool{}
		after := uuid.Nil
		for {
			page, cursor, pageErr := f.service.DiscoverNotificationSourcesPageInTransaction(
				t.Context(),
				tx,
				f.eventID,
				nil,
				after,
				100,
			)
			if pageErr != nil {
				return pageErr
			}
			if len(page) > 100 {
				return fmt.Errorf("unbounded native metadata page")
			}
			raw, _ := json.Marshal(page)
			if strings.Contains(string(raw), "PRIVATE") ||
				strings.Contains(string(raw), question.ID.String()) {
				return fmt.Errorf("private question data entered native metadata")
			}
			for _, item := range page {
				if seen[item.AnswerID] {
					return fmt.Errorf("duplicate source across pages")
				}
				seen[item.AnswerID] = true
			}
			if cursor == nil {
				break
			}
			after = *cursor
		}
		if len(seen) != 1005 {
			return fmt.Errorf("native publication history truncated: %d", len(seen))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.exec(
		`UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2`,
		f.eventID,
		f.first,
	)
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		people, e := f.service.ResolveNotificationAudienceInTransaction(
			t.Context(),
			tx,
			f.eventID,
			last,
			1,
		)
		if e != nil {
			return e
		}
		if len(people) != 2 || slices.Contains(people, f.first) {
			return fmt.Errorf("native current audience retained withdrawn person: %v", people)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
