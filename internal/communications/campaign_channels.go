package communications

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

var opaqueChannelRevision = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

var campaignChannels = []string{"in_app", "smtp", "teams"}

func (s Service) channelReadiness(
	ctx context.Context,
	eventID, actor uuid.UUID,
) []ChannelReadiness {
	result := make([]ChannelReadiness, 0, len(campaignChannels))
	for _, channel := range campaignChannels {
		r := ChannelReadiness{
			Channel:          channel,
			SupportsSelfTest: channel != "teams",
			Reason:           "not_configured",
			SelfTestReason:   "not_configured",
			SetupURL:         "#campaign-provider-setup",
		}
		if provider := s.Channels[channel]; provider != nil {
			current, err := provider.Readiness(ctx, eventID, actor)
			if err != nil {
				r.Reason = "readiness_unavailable"
				r.SelfTestReason = "readiness_unavailable"
			} else {
				r.Available = current.Available &&
					opaqueChannelRevision.MatchString(current.ConfigRevision)
				if r.Available {
					r.ConfigRevision = current.ConfigRevision
				}
				r.SelfTestAvailable = r.Available && r.SupportsSelfTest &&
					current.SelfTestAvailable &&
					s.Jobs != nil
				if allowed := map[string]string{"smtp": "#smtp-provider-setup", "teams": "#teams-workflow-setup", "in_app": "#in-app-provider-setup"}[channel]; current.SetupURL == allowed {
					r.SetupURL = allowed
				}
				if r.Available {
					r.Reason = "ready"
				}
				if r.SelfTestAvailable {
					r.SelfTestReason = "ready"
				} else if current.SelfTestReason == "address_unverified" {
					r.SelfTestReason = "address_unverified"
				}
			}
		}
		if !r.SupportsSelfTest {
			r.SelfTestReason = "broadcast_only"
		}
		result = append(result, r)
	}
	return result
}

func channelAllowed(channels []string, providers []ChannelReadiness) []string {
	blockers := []string{}
	for _, channel := range channels {
		found := slices.IndexFunc(
			providers,
			func(p ChannelReadiness) bool { return p.Channel == channel && p.Available },
		)
		if found < 0 {
			blockers = append(blockers, "provider_unavailable_"+channel)
		}
	}
	return blockers
}

func broadcastBlockers(t Template, a Audience, facts []dbgen.CommunicationFact) []string {
	if len(a.Roles) != 1 || a.Roles[0] != "attendee" || len(a.RegistrationStates) != 0 ||
		len(a.Teams) != 0 ||
		a.Outstanding != "" {
		return []string{"teams_requires_common_audience"}
	}
	if !slices.Contains(
		[]string{
			"preparation",
			"reminder",
			"challenge_release",
			"results",
			"postponement",
			"clarification",
			"rubric",
		},
		t.Purpose,
	) {
		return []string{"teams_requires_broadcast_content"}
	}
	used, _ := tokens(t.Subject + "\n" + t.Body)
	for _, token := range used {
		if !strings.HasPrefix(token, "event.") &&
			!slices.Contains(
				[]string{
					"challenge.title",
					"results.revision",
					"clarification.revision",
					"rubric.revision",
				},
				token,
			) {
			return []string{"teams_requires_broadcast_content"}
		}
	}
	for _, id := range []*uuid.UUID{t.FactID, a.FactID} {
		if id != nil && !slices.ContainsFunc(facts, func(f dbgen.CommunicationFact) bool {
			return f.ID == *id && f.OwnerID == nil && f.ResourceID == nil &&
				f.Status == "published" &&
				slices.Contains([]string{"results", "clarification", "rubric"}, f.Kind)
		}) {
			return []string{"teams_requires_broadcast_content"}
		}
	}
	return []string{}
}
