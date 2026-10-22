package rubric

import "github.com/google/uuid"

// Default provides a draft, never an implicit approval. Stable category IDs make
// repeated template rendering deterministic within its independent namespace.
func Default(mode string, namespace uuid.UUID) Definition {
	d := Definition{
		PublicDefinition: PublicDefinition{
			Name:               "Event judging rubric",
			Mode:               mode,
			RequiredJudgeCount: 1,
			Categories:         []Category{},
			Prizes:             []Prize{},
			SideQuests:         []SideQuest{},
		},
	}
	if mode == MentorQualification {
		d.FinalistCount = 3
		d.ReserveCount = 1
	}
	for _, spec := range []struct{ label, guidance, low, mid, high string }{
		{"Demo", "Show the competition entry working and explain the evidence.", "The core entry cannot be demonstrated.", "The main path is demonstrated with material gaps.", "A coherent end-to-end demonstration supports the claims."},
		{"Technical approach", "Explain technical decisions, tradeoffs and implementation evidence.", "The approach is unsupported or does not address the problem.", "The main technical approach works with identifiable limitations.", "The approach is sound, well explained and supported by evidence."},
		{"UX", "Assess the intended user's journey using demonstrated evidence.", "The intended user cannot complete the main task.", "The main user journey is usable with material friction.", "A clear, coherent user journey supports the intended task."},
		{"Feature fit", "Connect demonstrated capabilities to the published challenge.", "The demonstrated features do not address the challenge.", "Core features address the challenge with material gaps.", "Demonstrated features directly and convincingly address the challenge."},
		{"Innovation", "Explain what is distinctive and why it is useful for this challenge.", "No distinctive useful approach is evidenced.", "A useful adaptation or combination is demonstrated.", "A distinctive useful approach is convincingly demonstrated and explained."},
	} {
		d.Categories = append(
			d.Categories,
			Category{
				ID:       uuid.NewSHA1(namespace, []byte(spec.label)),
				Label:    spec.label,
				MinScore: 1,
				MaxScore: 10,
				Weight:   1,
				Guidance: spec.guidance,
				Anchors: []Anchor{
					{Score: 1, Description: spec.low},
					{Score: 5, Description: spec.mid},
					{Score: 10, Description: spec.high},
				},
			},
		)
	}
	return d
}
