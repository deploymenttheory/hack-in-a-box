package submissions

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// Report reads only the selected audience's formal submission view inside the
// caller's event transaction. Private drafts and review reasons are never used.
func (s Service) Report(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	audience string,
) (Page, error) {
	a, err := access.TeamPermissions(ctx, dbgen.New(tx), actor, eventID, teamID)
	if err != nil {
		return Page{}, fmt.Errorf("report team authority: %w", err)
	}
	if (audience == "organiser" && !a.Organiser) ||
		(audience == "mentor" && !a.Mentor && !a.Reviewer) ||
		(audience == "participant" && !a.Member) {
		return Page{}, access.ErrForbidden
	}
	v, err := s.scoped(ctx, tx, actor, eventID, teamID)
	if err != nil {
		return Page{}, err
	}
	switch audience {
	case "organiser":
		if !v.authority.Organiser {
			return Page{}, access.ErrForbidden
		}
	case "mentor":
		if !v.authority.Mentor && !v.authority.Reviewer {
			return Page{}, access.ErrForbidden
		}
		v.authority.Organiser, v.authority.Member, v.authority.Judge = false, false, false
		if !published(v.row, s.now()) || v.config.LiveStart.IsZero() ||
			s.now().Before(v.config.LiveStart) {
			return Page{}, access.ErrForbidden
		}
	case "participant":
		if !v.authority.Member {
			return Page{}, access.ErrForbidden
		}
		v.authority.Organiser, v.authority.Mentor, v.authority.Reviewer, v.authority.Judge = false, false, false, false
	default:
		return Page{}, access.ErrInvalid
	}
	return s.page(ctx, dbgen.New(tx), v)
}
