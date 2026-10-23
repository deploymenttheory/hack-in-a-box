// Package audit stores a deliberately narrow, append-only decision history.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type Action string

const (
	RepositoryAccessChanged       Action = "repository.access_changed"
	RepositoryProvisioningChanged Action = "repository.provisioning_changed"
	SurveyResponsesExported       Action = "survey.responses_exported"
	MediaChanged                  Action = "media.changed"
	MediaRevoked                  Action = "media.revoked"
	PhotoConsentGranted           Action = "photo_consent.granted"
	PhotoConsentRevoked           Action = "photo_consent.revoked"
	PhotoConsentUnknown           Action = "photo_consent.unknown"
	OutcomeChanged                Action = "outcome.changed"
	OutcomeArchivedUpdated        Action = "outcome.archived_updated"
	OutcomeProjectChanged         Action = "outcome.project_changed"
	OutcomeReviewChanged          Action = "outcome.review_changed"
)

const (
	GitHubActivityRecorded      Action = "github.activity_recorded"
	GitHubIntegrationConfigured Action = "github.integration_configured"
	GitHubIntegrationInstalled  Action = "github.integration_installed"
	SurveyChanged               Action = "survey.changed"
	SurveyResponded             Action = "survey.responded"
	SurveyReportChanged         Action = "survey.report_changed"
	SurveyPersonalRemoved       Action = "survey.personal_removed"
	FeedbackChanged             Action = "feedback.changed"
	ClarificationChanged        Action = "clarification.changed"
	RubricChanged               Action = "rubric.changed"
	CommunicationChanged        Action = "communication.changed"
	CredentialCreated           Action = "credential.created"          // #nosec G101 -- Audit action name, not a credential.
	CredentialReplaced          Action = "credential.replaced"         // #nosec G101 -- Audit action name, not a credential.
	CredentialAssigned          Action = "credential.assigned"         // #nosec G101 -- Audit action name, not a credential.
	CredentialRevoked           Action = "credential.revoked"          // #nosec G101 -- Audit action name, not a credential.
	CredentialMetadataChanged   Action = "credential.metadata_changed" // #nosec G101 -- Audit action name, not a credential.
	CredentialRotated           Action = "credential.rotated"          // #nosec G101 -- Audit action identifier, not a credential.
	CredentialChanged           Action = "credential.changed"          // #nosec G101 -- Audit action identifier, not a credential.
	CredentialRevealed          Action = "credential.revealed"         // #nosec G101 -- Audit action identifier, not a credential.
)

const (
	TeamsConnectionSaved       Action = "teams.connection_saved"
	TeamsConnectionRevoked     Action = "teams.connection_revoked"
	TeamsConnectionRotated     Action = "teams.connection_rotated"
	TeamsTestQueued            Action = "teams.test_queued"
	TeamsBroadcastQueued       Action = "teams.broadcast_queued"
	PresentationChanged        Action = "presentation.changed"
	DisplayPaired              Action = "display.paired"
	DisplayRevoked             Action = "display.revoked"
	ContentAcknowledged        Action = "content.acknowledged"
	ContentChanged             Action = "content.changed"
	ContentReleased            Action = "content.released"
	AdminBootstrapped          Action = "admin.bootstrapped"
	SignedIn                   Action = "identity.signed_in"
	FileUploaded               Action = "file.uploaded"
	OrganiserGranted           Action = "organiser.granted"
	PrivilegedAccessGranted    Action = "privileged_access.granted"
	AttendanceException        Action = "attendance.exception"
	MentorCoverageChanged      Action = "mentor.coverage_changed"
	ScorecardCheckpointChanged Action = "scorecard.checkpoint_changed"
	ScorecardChanged           Action = "scorecard.changed"
	ScorecardSignedOff         Action = "scorecard.signed_off"
	ModerationChanged          Action = "moderation.changed"
	MentorObservationChanged   Action = "mentor.observation_changed"
	MentorStatusChanged        Action = "mentor.status_changed"
	TeamFrozen                 Action = "team.frozen"
	TeamRebalanced             Action = "team.rebalanced"
	ChallengeApproved          Action = "challenge.approved"
	ChallengePostponed         Action = "challenge.postponed"
	SubmissionFinalized        Action = "submission.finalized"
	SubmissionReviewed         Action = "submission.reviewed"
	SubmissionOutcomeChanged   Action = "submission.outcome_changed"
	RoleGranted                Action = "role.granted"
	RoleRevoked                Action = "role.revoked"
	OwnerTransferred           Action = "owner.transferred"
	EventCreated               Action = "event.created"
	EventUpdated               Action = "event.updated"
	EventStageChanged          Action = "event.stage_changed"
	EventDecision              Action = "event.decision"
	ScheduleApproved           Action = "schedule.approved"
	ProfileCorrected           Action = "profile.corrected"
	ProfileExported            Action = "profile.exported"
	ProfileRemoved             Action = "profile.removed"
	PrivacyConfigured          Action = "privacy.configured"
	TemplateCreated            Action = "template.created"
	TemplateUpdated            Action = "template.updated"
	TemplatePublished          Action = "template.published"
	DefinitionsUpdated         Action = "definitions.updated"
	EventCloned                Action = "event.cloned"
	RegistrationChanged        Action = "registration.changed"
	PreparationChanged         Action = "preparation.changed"
	AttendanceChanged          Action = "attendance.changed"
	CateringConfigured         Action = "catering.configured"
	GitHubLinked               Action = "github.linked"
	GitHubRelinked             Action = "github.relinked"
	GitHubUnlinked             Action = "github.unlinked"
)

type Reason string

const (
	Bootstrap          Reason = "deployment_bootstrap"
	Authentication     Reason = "authentication"
	UserRequest        Reason = "user_request"
	OrganiserDecision  Reason = "organiser_decision"
	ThresholdException Reason = "post_publication_withdrawal"
	RetentionPolicy    Reason = "retention_policy"
	ConsentPolicy      Reason = "consent_policy"
	DeadlinePolicy     Reason = "deadline_policy"
)

// Metadata has no arbitrary strings/maps: credentials, brief text, accessibility
// needs and assessment prose cannot accidentally enter generic audit payloads.
type Metadata struct {
	Result           *string    `json:"result,omitempty"`
	ReferenceID      *uuid.UUID `json:"reference_id,omitempty"`
	PreviousRevision *int64     `json:"previous_revision,omitempty"`
	Revision         *int64     `json:"revision,omitempty"`
	Count            *int       `json:"count,omitempty"`
	Role             *string    `json:"role,omitempty"`
}
type Record struct {
	ActorID, EventID *uuid.UUID
	TargetID         uuid.UUID
	Action           Action
	Reason           Reason
	Metadata         Metadata
}

func Append(ctx context.Context, q *dbgen.Queries, r Record) error {
	switch r.Action {
	case RepositoryAccessChanged,
		RepositoryProvisioningChanged,
		SurveyResponsesExported,
		MediaChanged,
		MediaRevoked,
		PhotoConsentGranted,
		PhotoConsentRevoked,
		PhotoConsentUnknown,
		GitHubActivityRecorded,
		GitHubIntegrationConfigured,
		GitHubIntegrationInstalled,
		OutcomeChanged,
		OutcomeArchivedUpdated,
		OutcomeProjectChanged,
		OutcomeReviewChanged,
		SurveyChanged,
		SurveyResponded,
		SurveyReportChanged,
		SurveyPersonalRemoved,
		FeedbackChanged,
		ClarificationChanged,
		RubricChanged,
		TeamsConnectionSaved,
		TeamsConnectionRevoked,
		TeamsConnectionRotated,
		TeamsTestQueued,
		TeamsBroadcastQueued,
		PresentationChanged,
		DisplayPaired,
		DisplayRevoked,
		CommunicationChanged,
		CredentialCreated,
		CredentialReplaced,
		CredentialAssigned,
		CredentialRevoked,
		CredentialMetadataChanged,
		CredentialRotated,
		CredentialChanged,
		CredentialRevealed,
		MentorCoverageChanged,
		ScorecardCheckpointChanged,
		ScorecardChanged,
		ScorecardSignedOff,
		ModerationChanged,
		MentorObservationChanged,
		MentorStatusChanged,
		ContentAcknowledged,
		ContentChanged,
		ContentReleased,
		SubmissionReviewed,
		SubmissionOutcomeChanged,
		AttendanceChanged,
		CateringConfigured,
		PreparationChanged,
		AdminBootstrapped,
		SignedIn,
		FileUploaded,
		OrganiserGranted,
		PrivilegedAccessGranted,
		AttendanceException,
		TeamFrozen,
		TeamRebalanced,
		ChallengeApproved,
		ChallengePostponed,
		SubmissionFinalized,
		RoleGranted,
		RoleRevoked,
		OwnerTransferred,
		EventCreated,
		EventUpdated,
		EventStageChanged,
		EventDecision,
		ScheduleApproved,
		ProfileCorrected,
		ProfileExported,
		ProfileRemoved,
		PrivacyConfigured,
		TemplateCreated,
		TemplateUpdated,
		TemplatePublished,
		DefinitionsUpdated,
		EventCloned,
		RegistrationChanged,
		GitHubLinked,
		GitHubRelinked,
		GitHubUnlinked:
	default:
		return access.ErrInvalid
	}
	switch r.Reason {
	case Bootstrap,
		Authentication,
		UserRequest,
		OrganiserDecision,
		ThresholdException,
		RetentionPolicy, DeadlinePolicy, ConsentPolicy:
	default:
		return access.ErrInvalid
	}
	if r.TargetID == uuid.Nil {
		return access.ErrInvalid
	}
	if r.Metadata.Result != nil {
		switch *r.Metadata.Result {
		case "success", "denied", "unavailable", "not_found", "conflict":
		default:
			return access.ErrInvalid
		}
	}
	if r.Metadata.Role != nil {
		switch *r.Metadata.Role {
		case "organiser",
			"mentor",
			"judge",
			"mc",
			"logistics",
			"photographer",
			"early_brief_access":
		default:
			return access.ErrInvalid
		}
	}
	metadata, err := json.Marshal(r.Metadata)
	if err != nil {
		return fmt.Errorf("encode audit references: %w", err)
	}
	_, err = q.AppendAudit(
		ctx,
		dbgen.AppendAuditParams{
			ActorID:    r.ActorID,
			EventID:    r.EventID,
			TargetID:   r.TargetID,
			Action:     string(r.Action),
			ReasonCode: string(r.Reason),
			Metadata:   metadata,
		},
	)
	if err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	return nil
}

type Service struct {
	Queries *dbgen.Queries
	Policy  access.Policy
}

func (s Service) List(
	ctx context.Context,
	actor, event uuid.UUID,
	cursor int64,
	limit int32,
	export bool,
) ([]dbgen.AuditEvent, error) {
	action := access.AuditRead
	if export {
		action = access.AuditExport
	}
	if s.Policy == nil {
		return nil, access.ErrForbidden
	}
	if err := s.Policy.Authorize(ctx, actor, event, event, action); err != nil {
		return nil, fmt.Errorf("authorize: %w", err)
	}
	if cursor < 0 || limit < 1 || limit > 100 {
		return nil, access.ErrInvalid
	}
	rows, err := s.Queries.ListAudit(
		ctx,
		dbgen.ListAuditParams{EventID: &event, Sequence: cursor, Limit: limit},
	)
	if err != nil {
		return nil, fmt.Errorf("query audit events: %w", err)
	}
	return rows, nil
}
