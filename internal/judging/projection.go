package judging

import (
	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

// The immutable authored snapshot is not a lasting per-deliverable grant.
// Project references and their attached prose through the recipient's current
// native exact-revision document. Never mutate retained cards or summary facts.
func visibleDeliverables(document submissions.Document) map[uuid.UUID]bool {
	ids := make(map[uuid.UUID]bool, len(document.Evidence))
	for _, e := range document.Evidence {
		ids[e.DeliverableID] = true
	}
	return ids
}

func projectCard(card Card, document submissions.Document) Card {
	ids := visibleDeliverables(document)
	scores := make([]Score, 0, len(card.Scores))
	for _, score := range card.Scores {
		evidence := make([]Evidence, 0, len(score.Evidence))
		for _, item := range score.Evidence {
			if item.DeliverableID == nil || ids[*item.DeliverableID] {
				evidence = append(evidence, item)
			}
		}
		score.Evidence = evidence
		scores = append(scores, score)
	}
	card.Scores = scores
	return card
}

func projectSummary(summary FactualSummary, document submissions.Document) FactualSummary {
	ids := visibleDeliverables(document)
	facts := make([]SummaryFact, 0, len(summary.Facts))
	for _, fact := range summary.Facts {
		if ids[fact.DeliverableID] {
			facts = append(facts, fact)
		}
	}
	summary.Facts = facts
	return summary
}
