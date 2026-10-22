// Package rubric owns immutable assessment criteria and their common attendee
// publication. Assessment consumers bind records to exact approved run versions.
package rubric

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

const (
	AllTeams            = "all_teams"
	MentorQualification = "mentor_qualification"
)

type (
	Anchor struct {
		Score       int    `json:"score"`
		Description string `json:"description"`
	}
	Category struct {
		ID       uuid.UUID `json:"id"`
		Label    string    `json:"label"`
		MinScore int       `json:"min_score"`
		MaxScore int       `json:"max_score"`
		Weight   int       `json:"weight"`
		Guidance string    `json:"guidance"`
		Anchors  []Anchor  `json:"anchors"`
	}
)

type (
	Prize struct {
		ID          uuid.UUID  `json:"id"`
		Name        string     `json:"name"`
		Description string     `json:"description"`
		CategoryID  *uuid.UUID `json:"category_id"`
	}
	// SideQuest supplies optional evidence only. There is deliberately no automatic
	// points, score or weight field for consumers to add to assessment totals.
	SideQuest struct {
		ID               uuid.UUID `json:"id"`
		Title            string    `json:"title"`
		EvidenceGuidance string    `json:"evidence_guidance"`
	}
	// PublicDefinition is the identical common snapshot every confirmed attendee
	// receives. Private judge instructions never enter this type or its digest.
	PublicDefinition struct {
		Name               string      `json:"name"`
		Mode               string      `json:"mode"`
		Categories         []Category  `json:"categories"`
		RequiredJudgeCount int         `json:"required_judge_count"`
		FinalistCount      int         `json:"finalist_count"`
		ReserveCount       int         `json:"reserve_count"`
		Prizes             []Prize     `json:"prizes"`
		SideQuests         []SideQuest `json:"side_quests"`
	}
)

type Definition struct {
	PublicDefinition
	PrivateJudgeGuidance string `json:"private_judge_guidance"`
}

// Reference identifies the immutable approved version; Digest covers only the
// public snapshot, while the version retains its separate private definition.
type Reference struct {
	ID      uuid.UUID `json:"id"`
	Version int64     `json:"version"`
	Digest  string    `json:"digest"`
}

func (d Definition) Validate() error {
	if strings.TrimSpace(d.Name) == "" || len(d.Name) > 160 ||
		!slices.Contains([]string{AllTeams, MentorQualification}, d.Mode) ||
		len(d.Categories) == 0 ||
		len(d.Categories) > 20 ||
		d.RequiredJudgeCount < 1 ||
		d.RequiredJudgeCount > 50 ||
		len(d.PrivateJudgeGuidance) > 10000 ||
		len(d.Prizes) > 50 ||
		len(d.SideQuests) > 50 {
		return access.ErrInvalid
	}
	if (d.Mode == AllTeams && (d.FinalistCount != 0 || d.ReserveCount != 0)) ||
		(d.Mode == MentorQualification && (d.FinalistCount < 1 || d.FinalistCount > 100 || d.ReserveCount < 0 || d.ReserveCount > 100)) {
		return access.ErrInvalid
	}
	ids, err := validateCategories(d.Categories)
	if err != nil {
		return err
	}
	return validateExtras(d.Prizes, d.SideQuests, ids)
}

func (d Definition) PublicDigest() string {
	encoded, _ := json.Marshal(d.PublicDefinition)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func validateCategories(categories []Category) (map[uuid.UUID]bool, error) {
	ids := map[uuid.UUID]bool{}
	for _, c := range categories {
		if c.ID == uuid.Nil || ids[c.ID] || strings.TrimSpace(c.Label) == "" ||
			len(c.Label) > 160 ||
			c.MinScore < 0 ||
			c.MaxScore <= c.MinScore ||
			c.MaxScore > 1000 ||
			c.Weight < 1 ||
			c.Weight > 1000 ||
			len(c.Guidance) > 10000 ||
			len(c.Anchors) > 30 {
			return nil, access.ErrInvalid
		}
		ids[c.ID] = true
		scores := map[int]bool{}
		for _, a := range c.Anchors {
			if a.Score < c.MinScore || a.Score > c.MaxScore || scores[a.Score] ||
				strings.TrimSpace(a.Description) == "" ||
				len(a.Description) > 2000 {
				return nil, access.ErrInvalid
			}
			scores[a.Score] = true
		}
	}
	return ids, nil
}

func validateExtras(prizes []Prize, quests []SideQuest, ids map[uuid.UUID]bool) error {
	prizeIDs := map[uuid.UUID]bool{}
	for _, p := range prizes {
		if p.ID == uuid.Nil || prizeIDs[p.ID] || strings.TrimSpace(p.Name) == "" ||
			len(p.Name) > 160 ||
			len(p.Description) > 10000 ||
			(p.CategoryID != nil && !ids[*p.CategoryID]) {
			return access.ErrInvalid
		}
		prizeIDs[p.ID] = true
	}
	sideQuestIDs := map[uuid.UUID]bool{}
	for _, quest := range quests {
		if quest.ID == uuid.Nil || sideQuestIDs[quest.ID] || strings.TrimSpace(quest.Title) == "" ||
			len(quest.Title) > 160 ||
			len(quest.EvidenceGuidance) > 10000 {
			return access.ErrInvalid
		}
		sideQuestIDs[quest.ID] = true
	}
	return nil
}
