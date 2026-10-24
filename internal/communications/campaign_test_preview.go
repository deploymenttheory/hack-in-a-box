package communications

import (
	"context"
	"encoding/hex"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/mailcontact"
)

// SelfTestPreview is an ephemeral current review, never a send acknowledgement.
// The verified mailbox participates only in the private hash; it is not returned.
type SelfTestPreview struct {
	Revision          int64     `json:"revision"`
	CampaignVersion   int64     `json:"campaign_version"`
	ActorID           uuid.UUID `json:"actor_id"`
	Channel           string    `json:"channel"`
	Subject           string    `json:"subject"`
	PlainText         string    `json:"plain_text"`
	IncludeCalendar   bool      `json:"include_calendar"`
	ProviderRevision  string    `json:"provider_revision"`
	ReviewFingerprint string    `json:"review_fingerprint"`
}

func validReviewFingerprint(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func selfTestReviewBinding(ctx context.Context, q *dbgen.Queries, campaign Campaign, message TestMessage, source string) (string, error) {
	mailbox := ""
	if message.Channel == "smtp" {
		// Account/contact writers use this order. Retain the reviewed mailbox
		// through the worker's immutable SMTP attempt preparation barrier.
		if _, err := q.LockMailPerson(ctx, message.ActorID); err != nil {
			return "", contextError("lock reviewed test account", err)
		}
		if _, err := q.LockPersonEmailContact(ctx, message.ActorID); err != nil {
			return "", contextError("lock reviewed test destination", err)
		}
		var err error
		mailbox, err = mailcontact.VerifiedMailboxInTransaction(ctx, q, message.ActorID)
		if err != nil {
			return "", contextError("read reviewed test destination", err)
		}
	}
	// Normalize only the generated operation identity. Render once at enqueue and
	// validate exactly that snapshot; do not render a second competing message.
	message.ID = uuid.Nil
	message.ReviewedTestBinding = ""
	return digest(struct {
		Campaign        Campaign
		Message         TestMessage
		Source, Mailbox string
	}{campaign, message, source, mailbox}), nil
}

// PreviewSelfTest uses the exact organiser-only renderer, with current write
// authority, without saving a preview, audit row, revision, operation or job.
func (s Service) PreviewSelfTest(ctx context.Context, actor, eventID, campaignID uuid.UUID, channel string) (SelfTestPreview, error) {
	var result SelfTestPreview
	if !slices.Contains([]string{"smtp", "in_app"}, channel) {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := campaignEvent(ctx, q, actor, eventID, true)
		if err != nil {
			return err
		}
		campaign, err := loadCampaign(ctx, q, eventID, campaignID)
		if err != nil {
			return err
		}
		if campaign.Cancelled || !slices.Contains(campaign.Definition.Channels, channel) {
			return access.ErrConflict
		}
		ready := s.channelReadiness(ctx, eventID, actor)
		if slices.IndexFunc(ready, func(value ChannelReadiness) bool { return value.Channel == channel && value.SelfTestAvailable }) < 0 {
			return ErrUnresolved
		}
		message, _, source, err := s.selfMessage(ctx, q, row, actor, campaign, uuid.Nil, channel)
		if err != nil {
			return err
		}
		binding, err := selfTestReviewBinding(ctx, q, campaign, message, source)
		if err != nil {
			return err
		}
		result = SelfTestPreview{Revision: row.Revision, CampaignVersion: campaign.Version, ActorID: actor, Channel: channel, Subject: message.Subject, PlainText: message.PlainText, IncludeCalendar: message.IncludeCalendar, ProviderRevision: message.ProviderRevision, ReviewFingerprint: binding}
		return nil
	})
	return result, contextError("preview organiser test", err)
}
