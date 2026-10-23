package access

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// ContentAudience is evaluated against current grants and published membership,
// never the membership snapshot at approval time.
type ContentAudience struct {
	Confirmed bool        `json:"confirmed"`
	Roles     []string    `json:"roles"`
	Teams     []uuid.UUID `json:"teams"`
}

func ContentReader(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
) (roles []string, confirmed bool, err error) {
	p, err := q.Person(ctx, actor)
	if err != nil || !p.Active {
		return nil, false, ErrForbidden
	}
	roles, err = q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
	if err != nil {
		return nil, false, fmt.Errorf("read content roles: %w", err)
	}
	claim, err := q.CompetitionState(
		ctx,
		dbgen.CompetitionStateParams{EventID: eventID, PersonID: actor},
	)
	if err != nil {
		return nil, false, fmt.Errorf("read content attendance: %w", err)
	}
	return roles, claim == "confirmed", nil
}

// ContentVersionForReader returns zero when the item must be undiscoverable.
// Preview is an explicit privileged operation, not an audience simulation grant.
func ContentVersionForReader(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	item dbgen.ContentItem,
	preview bool,
) (int64, error) {
	roles, confirmed, err := ContentReader(ctx, q, actor, item.EventID)
	if err != nil {
		return 0, err
	}
	organiser := slices.Contains(roles, "organiser")
	if preview {
		if organiser ||
			(item.Kind != "page" && slices.Contains(roles, "early_brief_access") && item.State != "withdrawn") {
			return item.Version, nil
		}
		return 0, nil
	}
	if !item.ReleasedVersion.Valid || item.State == "withdrawn" {
		return 0, nil
	}
	if organiser {
		return item.ReleasedVersion.Int64, nil
	}
	v, err := q.ContentVersion(
		ctx,
		dbgen.ContentVersionParams{
			EventID:   item.EventID,
			ContentID: item.ID,
			Version:   item.ReleasedVersion.Int64,
		},
	)
	if err != nil {
		return 0, fmt.Errorf("read content audience version: %w", err)
	}
	var doc struct {
		Audience ContentAudience `json:"audience"`
	}
	if err = json.Unmarshal(v.Document, &doc); err != nil {
		return 0, fmt.Errorf("read content audience: %w", err)
	}
	allowed, err := ContentAudienceAllows(
		ctx,
		q,
		actor,
		item.EventID,
		doc.Audience,
		roles,
		confirmed,
	)
	if err != nil {
		return 0, err
	}
	if allowed {
		return v.Version, nil
	}
	return 0, nil
}

func (p EventPolicy) contentFile(
	ctx context.Context,
	actor uuid.UUID,
	file dbgen.File,
	organiser bool,
) error {
	items, err := p.Queries.ContentFileItems(
		ctx,
		dbgen.ContentFileItemsParams{EventID: file.EventID, FileID: file.ID},
	)
	if err != nil {
		return fmt.Errorf("read attachment scope: %w", err)
	}
	if len(items) == 0 && organiser {
		return nil
	}
	for _, item := range items {
		if item.State == "withdrawn" {
			continue
		}
		for _, preview := range []bool{false, true} {
			version, err := ContentVersionForReader(ctx, p.Queries, actor, item, preview)
			if err != nil {
				return err
			}
			if version == 0 {
				continue
			}
			linked, err := p.Queries.ContentFileReferenced(
				ctx,
				dbgen.ContentFileReferencedParams{
					EventID:   file.EventID,
					ContentID: item.ID,
					Version:   version,
					FileID:    file.ID,
				},
			)
			if err != nil {
				return fmt.Errorf("check attachment version: %w", err)
			}
			if linked {
				return nil
			}
		}
	}
	return ErrForbidden
}

// ContentAudienceAllows is the shared current-role/current-published-team predicate.
// Callers must obtain roles and confirmed through ContentReader in the same transaction.
func ContentAudienceAllows(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	audience ContentAudience,
	roles []string,
	confirmed bool,
) (bool, error) {
	if (audience.Confirmed && confirmed) ||
		slices.ContainsFunc(
			roles,
			func(r string) bool { return slices.Contains(audience.Roles, r) },
		) {
		return true, nil
	}
	if confirmed && len(audience.Teams) > 0 {
		memberships, err := q.ContentTeamMember(
			ctx,
			dbgen.ContentTeamMemberParams{
				EventID:  eventID,
				PersonID: actor,
				TeamIds:  audience.Teams,
			},
		)
		if err != nil {
			return false, fmt.Errorf("read content team audience: %w", err)
		}
		if memberships {
			return true, nil
		}
	}
	return false, nil
}
