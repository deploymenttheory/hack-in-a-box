// Package githubwebhooks accepts authenticated operational activity. It has no
// submission, assessment or provider mutation capability.
package githubwebhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v89/github"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

const MaxBodyBytes int64 = 1024 * 1024

var shaPattern = regexp.MustCompile(`^[a-fA-F0-9]{40}([a-fA-F0-9]{24})?$`)

func validSignature(raw, secret []byte, signature string) bool {
	if len(signature) != 71 || !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	supplied, err := hex.DecodeString(signature[7:])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(raw)
	return hmac.Equal(mac.Sum(nil), supplied)
}

type selected struct {
	repositoryID, organizationID, installationID int64
	kind, action, ref, after                     string
	memberID                                     int64
	providerAt                                   pgtype.Timestamptz
}

// SDK event types are used only after authentication. Commit messages, private
// repository prose, sender identities and full payloads never reach persistence.
func selectPayload(kind string, raw []byte, received time.Time) (selected, error) {
	var out selected
	out.kind = kind
	switch kind {
	case "push":
		var p github.PushEvent
		if err := json.Unmarshal(raw, &p); err != nil {
			return out, access.ErrInvalid
		}
		out.repositoryID = p.GetRepo().GetID()
		out.organizationID = p.GetRepo().GetOwner().GetID()
		out.installationID = p.GetInstallation().GetID()
		out.ref = p.GetRef()
		out.after = p.GetAfter()
		if !strings.HasPrefix(out.ref, "refs/") || len(out.ref) > 1024 ||
			strings.ContainsAny(out.ref, "\x00\r\n") ||
			!shaPattern.MatchString(out.after) {
			return out, access.ErrInvalid
		}
		// Repository pushed_at is provider metadata. Commit authored timestamps are
		// not delivery order. No webhook is ever treated as a current branch head.
		if at := p.GetRepo().GetPushedAt(); !at.IsZero() {
			if at.After(received.Add(5 * time.Minute)) {
				return out, access.ErrInvalid
			}
			out.providerAt = pgtype.Timestamptz{Time: at.UTC(), Valid: true}
		}
	case "member":
		var p github.MemberEvent
		if err := json.Unmarshal(raw, &p); err != nil {
			return out, access.ErrInvalid
		}
		out.repositoryID = p.GetRepo().GetID()
		out.organizationID = p.GetRepo().GetOwner().GetID()
		out.installationID = p.GetInstallation().GetID()
		out.action = p.GetAction()
		out.memberID = p.GetMember().GetID()
		if out.memberID <= 0 ||
			(out.action != "added" && out.action != "removed" && out.action != "edited") {
			return out, access.ErrInvalid
		}
		// GitHub supplies no authoritative occurrence time for member deliveries.
		// Leave provider_at null rather than inventing ordering from received time.
	default:
		return out, access.ErrInvalid
	}
	if out.repositoryID <= 0 || out.organizationID <= 0 || out.installationID <= 0 {
		return out, access.ErrInvalid
	}
	return out, nil
}
