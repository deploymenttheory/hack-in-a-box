package moderation

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestDelivery38PreservesNative37HuddleCardsFeedbackAndPrivateQuestion(t *testing.T) {
	ctx := t.Context()
	pool := testdb.New(t, false)
	if err := database.Migrate(ctx, pool, 37); err != nil {
		t.Fatal(err)
	}
	f := setupPool(t, pool)
	f.submit(f.checkpoint, f.primary.ID, true)
	f.submit(f.checkpoint, f.second.ID, true)
	native, feedbackID, _ := f.nativeAction()
	huddles := f.huddles()
	h := f.create(huddles, f.checkpoint)
	questions := clarification.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	question, err := questions.Ask(
		ctx,
		f.member.ID,
		f.event,
		clarification.QuestionInput{
			TeamID:    f.team,
			CommandID: uuid.New(),
			Topic:     "rubric",
			Body:      "PRIVATE actual37 question retained through delivery upgrade",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	before := priorDataSnapshot(t, pool)
	if err = database.Migrate(ctx, pool, 38); err != nil {
		t.Fatal(err)
	}
	after := priorDataSnapshot(t, pool)
	for table, value := range before {
		if after[table] != value {
			t.Fatal("37→38 changed retained native data", table)
		}
	}
	if _, err = huddles.Read(ctx, f.owner.ID, f.event, f.team, h.Definition.ID); err != nil {
		t.Fatal("native huddle unreadable after upgrade", err)
	}
	if _, err = native.Read(ctx, f.member.ID, f.event, feedbackID); err != nil {
		t.Fatal("native feedback unreadable after upgrade", err)
	}
	if _, err = questions.Question(ctx, f.member.ID, f.event, question.ID); err != nil {
		t.Fatal("private native question unreadable after upgrade", err)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}
