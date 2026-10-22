package content

import (
	"context"
	"fmt"
	"net/mail"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type GuidanceItem struct {
	ID                      uuid.UUID        `json:"id"`
	Purpose                 string           `json:"purpose"`
	Title                   string           `json:"title"`
	Version                 int64            `json:"version"`
	Contacts                []Contact        `json:"contacts"`
	RequireAcknowledgement  bool             `json:"require_acknowledgement"`
	AcknowledgedAt          *time.Time       `json:"acknowledged_at"`
	PreviousAcknowledgement *Acknowledgement `json:"previous_acknowledgement"`
}
type Guidance struct {
	Items     []GuidanceItem `json:"items"`
	Warnings  []event.Issue  `json:"warnings"`
	CanManage bool           `json:"can_manage"`
}

func contactURL(raw string) bool {
	if len(raw) > 2048 || strings.TrimSpace(raw) != raw {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil {
		return false
	}
	switch parsed.Scheme {
	case "http", "https":
		return parsed.Hostname() != ""
	case "mailto":
		if parsed.RawQuery != "" || parsed.Fragment != "" {
			return false
		}
		address, err := mail.ParseAddress(parsed.Opaque)
		return err == nil && address.Address == parsed.Opaque
	default:
		return false
	}
}

func validateGuidance(input Input) error {
	d := input.Document
	if !slices.Contains([]string{"", "rules", "support", "conduct"}, d.Purpose) ||
		len(d.Contacts) > 10 {
		return access.ErrInvalid
	}
	if input.Kind != "page" && (d.Purpose != "" || len(d.Contacts) > 0) {
		return access.ErrInvalid
	}
	if len(d.Contacts) > 0 && d.Purpose != "support" && d.Purpose != "conduct" {
		return access.ErrInvalid
	}
	for _, contact := range d.Contacts {
		if len(contact.Label) > 160 || len(contact.URL) > 2048 {
			return access.ErrInvalid
		}
		if contact.URL != "" && !strings.Contains(contact.URL, "{{") && !contactURL(contact.URL) {
			return access.ErrInvalid
		}
	}
	return nil
}

func approveGuidance(d Document) error {
	if d.Purpose == "rules" && !d.RequireAcknowledgement {
		return blocked(
			"rules_acknowledgement_required",
			"Rules must require a personal acknowledgement of each released version.",
		)
	}
	if d.Purpose != "support" && d.Purpose != "conduct" {
		return nil
	}
	if len(d.Contacts) == 0 {
		return blocked(
			"contact_configuration_required",
			"Configure a support or conduct-reporting contact before approval.",
		)
	}
	for _, contact := range d.Contacts {
		if strings.TrimSpace(contact.Label) == "" || strings.Contains(contact.Label, "{{") ||
			!contactURL(contact.URL) {
			return blocked(
				"contact_configuration_required",
				"Resolve each contact label and approved contact route before approval.",
			)
		}
	}
	return nil
}

// Guidance returns released native-page summaries only. It cannot select a
// challenge/brief or acquire privileged preview authority by changing a purpose.
// Missing common contacts are organiser warnings, never competition stage gates.
func (s Service) Guidance(ctx context.Context, actor, eventID uuid.UUID) (Guidance, error) {
	result := Guidance{Items: []GuidanceItem{}, Warnings: []event.Issue{}}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock guidance event: %w", err)
		}
		roles, confirmed, err := access.ContentReader(ctx, q, actor, eventID)
		if err != nil {
			return fmt.Errorf("authorize guidance: %w", err)
		}
		if !confirmed && len(roles) == 0 {
			return access.ErrForbidden
		}
		organiser := slices.Contains(roles, "organiser")
		result.CanManage = organiser && row.WritesAllowed
		items, err := q.ContentItems(
			ctx,
			dbgen.ContentItemsParams{EventID: eventID, Cursor: uuid.Nil, PageSize: 1001},
		)
		if err != nil {
			return fmt.Errorf("load guidance candidates: %w", err)
		}
		if len(items) > 1000 {
			return access.ErrConflict
		}
		common := map[string]bool{}
		for _, item := range items {
			if item.Kind != "page" || !item.ReleasedVersion.Valid || item.State == "withdrawn" ||
				!item.ReleasedAt.Valid || item.ReleasedAt.Time.After(s.now()) {
				continue
			}
			version, versionErr := access.ContentVersionForReader(ctx, q, actor, item, false)
			if versionErr != nil {
				return fmt.Errorf("authorize guidance resource: %w", versionErr)
			}
			if version == 0 {
				continue
			}
			d, documentErr := document(ctx, q, item, version)
			if documentErr != nil {
				return documentErr
			}
			if d.Purpose == "" || validateGuidance(Input{Kind: "page", Document: d}) != nil ||
				approveGuidance(d) != nil {
				continue
			}
			page, projectErr := projection(ctx, q, actor, item, false, 0)
			if projectErr != nil {
				return projectErr
			}
			contacts := d.Contacts
			if contacts == nil {
				contacts = []Contact{}
			}
			result.Items = append(
				result.Items,
				GuidanceItem{
					ID:                      item.ID,
					Purpose:                 d.Purpose,
					Title:                   d.Title,
					Version:                 version,
					Contacts:                contacts,
					RequireAcknowledgement:  d.RequireAcknowledgement,
					AcknowledgedAt:          page.AcknowledgedAt,
					PreviousAcknowledgement: page.PreviousAcknowledgement,
				},
			)
			if d.Audience.Confirmed {
				common[d.Purpose] = true
			}
		}
		if organiser {
			for _, kind := range []string{"support", "conduct"} {
				if !common[kind] {
					result.Warnings = append(
						result.Warnings,
						event.Issue{
							Field:   "content",
							Code:    "missing_" + kind + "_contact",
							Message: "Publish a configured " + kind + " contact for confirmed competitors.",
						},
					)
				}
			}
		}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("read event guidance: %w", err)
	}
	return result, nil
}
