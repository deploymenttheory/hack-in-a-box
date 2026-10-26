package loadprofile

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

// ApproveChallenge authors and separately approves the actual common source.
// Publication is left to the running shared worker at the native T-7 instant;
// this helper cannot directly release content or manufacture event readiness.
func (p *NativePreparer) ApproveChallenge(
	ctx context.Context,
	owner *Session,
	event, run uuid.UUID,
) (content.Page, error) {
	base := "/api/v1/events/" + event.String()
	definition := rubric.Default(rubric.AllTeams, event)
	definition.RequiredJudgeCount = 5
	definition.PrivateJudgeGuidance = "PRIVATE load fixture guidance " + run.String()
	zero := int64(0)
	saved, err := prepareCall[rubric.Configuration](
		ctx,
		p,
		owner,
		http.MethodPost,
		base+"/rubric/configuration",
		&zero,
		map[string]any{
			"action":     "save",
			"definition": definition,
			"reason":     "Synthetic release fixture exact common rubric",
		},
	)
	if err != nil {
		return content.Page{}, err
	}
	approved, err := prepareCall[rubric.Configuration](
		ctx,
		p,
		owner,
		http.MethodPost,
		base+"/rubric/configuration",
		&saved.Revision,
		map[string]any{
			"action":  "approve",
			"version": saved.Reference.Version,
			"reason":  "Reviewed native common rubric version",
		},
	)
	if err != nil {
		return content.Page{}, err
	}
	if !approved.Approved || approved.Reference != saved.Reference {
		return content.Page{}, fmt.Errorf(
			"%w: original common rubric approval lost exact reference",
			errCohort,
		)
	}
	input := content.Input{Kind: "challenge", Document: content.Document{
		Title:    "Synthetic load challenge " + event.String(),
		Markdown: "Build and demonstrate a synthetic service with explicit outcome evidence.",
		Audience: access.ContentAudience{
			Confirmed: true,
			Roles:     []string{},
			Teams:     []uuid.UUID{},
		},
		Attachments:            []uuid.UUID{},
		Rubric:                 &approved.Reference,
		RequireAcknowledgement: true,
	}}
	id := uuid.New()
	path := base + "/content/" + id.String()
	page, err := prepareCall[content.Page](ctx, p, owner, http.MethodPut, path, &zero, input)
	if err != nil {
		return page, err
	}
	for _, action := range []string{"review", "approve"} {
		page, err = prepareCall[content.Page](
			ctx,
			p,
			owner,
			http.MethodPost,
			path+"/actions",
			&page.Revision,
			map[string]string{
				"action": action,
				"reason": "Reviewed guarded synthetic common challenge release",
			},
		)
		if err != nil {
			return page, err
		}
	}
	if page.ID != id || page.Kind != "challenge" || page.State != "approved" ||
		page.ReleasedAt != nil {
		return page, fmt.Errorf(
			"%w: original challenge approval changed source or claimed early publication",
			errCohort,
		)
	}
	if page.Document.Rubric == nil || *page.Document.Rubric != approved.Reference {
		return page, fmt.Errorf("%w: original challenge approval changed exact rubric", errCohort)
	}
	return page, nil
}
