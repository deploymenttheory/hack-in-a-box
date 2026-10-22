package rubric

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// templateDefault reads the event's independent cloned public defaults. It
// grants no approval and never imports private judging instructions from a
// reusable template. Legacy templates remain drafts with their original range.
func templateDefault(ctx context.Context, q *dbgen.Queries, eventID uuid.UUID) (Definition, error) {
	row, err := q.EventDefinitions(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Default(AllTeams, eventID), nil
	}
	if err != nil {
		return Definition{}, fmt.Errorf("read event rubric defaults: %w", err)
	}
	var definitions struct {
		Rubrics []struct {
			Name               string      `json:"name"`
			Mode               string      `json:"mode"`
			RequiredJudgeCount int         `json:"required_judge_count"`
			FinalistCount      int         `json:"finalist_count"`
			ReserveCount       int         `json:"reserve_count"`
			Criteria           []Category  `json:"criteria"`
			Prizes             []Prize     `json:"prizes"`
			SideQuests         []SideQuest `json:"side_quests"`
		} `json:"rubrics"`
	}
	if err = json.Unmarshal(row.Definitions, &definitions); err != nil {
		return Definition{}, fmt.Errorf("decode event rubric defaults: %w", err)
	}
	if len(definitions.Rubrics) == 0 {
		return Default(AllTeams, eventID), nil
	}
	if len(definitions.Rubrics) != 1 {
		return Definition{}, blocked(
			"rubric_defaults_ambiguous",
			"Choose one common rubric explicitly before approving the challenge.",
		)
	}
	source := definitions.Rubrics[0]
	if source.Mode == "" {
		source.Mode = AllTeams
		source.RequiredJudgeCount = 1
	}
	if source.Prizes == nil {
		source.Prizes = []Prize{}
	}
	if source.SideQuests == nil {
		source.SideQuests = []SideQuest{}
	}
	for i := range source.Criteria {
		if source.Criteria[i].Anchors == nil {
			source.Criteria[i].Anchors = []Anchor{}
		}
	}
	d := Definition{
		PublicDefinition: PublicDefinition{
			Name:               source.Name,
			Mode:               source.Mode,
			RequiredJudgeCount: source.RequiredJudgeCount,
			FinalistCount:      source.FinalistCount,
			ReserveCount:       source.ReserveCount,
			Categories:         source.Criteria,
			Prizes:             source.Prizes,
			SideQuests:         source.SideQuests,
		},
	}
	if err = d.Validate(); err != nil {
		return Definition{}, blocked(
			"invalid_rubric_defaults",
			"Review the cloned rubric defaults before approving the challenge.",
		)
	}
	return d, nil
}
