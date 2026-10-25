package submissionfreeze

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestOriginalDecisionACKUsesCurrentProseAndTargetVisibility(t *testing.T) {
	pool := testdb.New(t, true)
	t.Run("organiser becomes assigned mentor", func(t *testing.T) {
		f := newFixture(t, pool)
		f.submit("repository")
		f.now = f.cfg.SubmissionCutoff
		f.capture()
		p := f.read(f.owner)
		command := f.decision(p, "reconcile")
		command.Reason = "PRIVATE-ORGANISER-REASON"
		command.ExternalEvidence = "PRIVATE-EXTERNAL-PROVENANCE"
		ack, err := f.service.Change(t.Context(), f.owner, f.event, f.team, command)
		if err != nil {
			t.Fatal(err)
		}
		var original []byte
		if err = pool.QueryRow(t.Context(), "SELECT result FROM submission_freeze_commands WHERE event_id=$1 AND request_id=$2", f.event, command.RequestID).
			Scan(&original); err != nil {
			t.Fatal(err)
		}
		f.exec(
			"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')",
			f.event,
			f.owner,
		)
		f.exec(
			"INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,'team')",
			f.event,
			f.team,
		)
		f.exec(
			"INSERT INTO resource_assignments(event_id,resource_id,person_id) VALUES($1,$2,$3)",
			f.event,
			f.team,
			f.owner,
		)
		f.exec(
			"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
			f.event,
			f.owner,
		)
		fresh := f.read(f.owner)
		if fresh.Current == nil || fresh.Current.Reason != "" ||
			fresh.Current.ExternalEvidence != "" {
			t.Fatal("fresh projection unsafe", fresh)
		}
		recovery, err := f.service.OriginalReceipt(
			t.Context(),
			f.owner,
			f.event,
			f.team,
			command.RequestID,
		)
		if err != nil {
			t.Fatal(err)
		}
		replayed, err := f.service.Change(t.Context(), f.owner, f.event, f.team, command)
		if err != nil {
			t.Fatal(err)
		}
		for _, receipt := range []*Receipt{recovery.Receipt, &replayed} {
			encoded, _ := json.Marshal(receipt)
			if receipt == nil || receipt.RequestID != ack.RequestID ||
				receipt.Fingerprint != ack.Fingerprint ||
				receipt.Decision == nil ||
				strings.Contains(string(encoded), "PRIVATE-") {
				t.Fatal(
					"current mentor recovered private organiser decision prose",
					string(encoded),
				)
			}
		}
		var retained []byte
		if err = pool.QueryRow(t.Context(), "SELECT result FROM submission_freeze_commands WHERE event_id=$1 AND request_id=$2", f.event, command.RequestID).
			Scan(&retained); err != nil ||
			string(original) != string(retained) {
			t.Fatal("projection rewrote original ACK", err)
		}
	})
	t.Run(
		"file-only original cannot expose mentor-only amendment to current judge",
		func(t *testing.T) {
			f := newFixture(t, pool)
			f.submit("url")
			f.now = f.cfg.SubmissionCutoff
			f.capture()
			repoURL := "https://github.com/synthetic-org/freeze-" + f.team.String()
			replace := func(audience string) {
				t.Helper()
				defs := templates.EmptyDefinitions()
				defs.Deliverables = []templates.Deliverable{
					{
						ID:            f.deliverable,
						Title:         "Scoped repository",
						Required:      true,
						EvidenceKinds: []string{"repository"},
						Audience:      []string{audience},
					},
				}
				raw, _ := json.Marshal(defs)
				f.exec(
					"UPDATE event_definitions SET definitions=$2 WHERE event_id=$1",
					f.event,
					raw,
				)
				s := native(*f.service)
				p, err := s.Read(t.Context(), f.owner, f.event, f.team)
				if err != nil {
					t.Fatal(err)
				}
				_, err = s.Submit(
					t.Context(),
					f.owner,
					f.event,
					f.team,
					p.Revision,
					submissions.Command{
						Reason: "Audited native correction to " + audience + " final",
						Evidence: []submissions.Evidence{
							{
								DeliverableID: f.deliverable,
								Kind:          "repository",
								Label:         "Current " + audience + " reference",
								URL:           repoURL,
							},
						},
					},
				)
				if err != nil {
					t.Fatal(err)
				}
			}
			replace("mentor")
			p := f.read(f.owner)
			command := f.decision(p, "amend")
			command.Reason = "PRIVATE-MENTOR-AMENDMENT"
			command.ExternalEvidence = "PRIVATE-MENTOR-PROVENANCE"
			ack, err := f.service.Change(t.Context(), f.owner, f.event, f.team, command)
			if err != nil {
				t.Fatal(err)
			}
			replace("judge")
			entry := p.Source.EntryID
			f.exec(
				"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'judge')",
				f.event,
				f.owner,
			)
			f.exec(
				"INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,'assessment')",
				f.event,
				entry,
			)
			f.exec(
				"INSERT INTO resource_assignments(event_id,resource_id,person_id) VALUES($1,$2,$3)",
				f.event,
				entry,
				f.owner,
			)
			f.exec(
				"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
				f.event,
				f.owner,
			)
			visible := f.read(f.owner)
			encoded, _ := json.Marshal(visible)
			if visible.Status != "native_revision_mismatch" || visible.Current != nil ||
				len(visible.Decisions) != 0 ||
				strings.Contains(string(encoded), strings.Repeat("a", 40)) {
				t.Fatal("private mentor amendment leaked through file-only source", string(encoded))
			}
			recovery, err := f.service.OriginalReceipt(
				t.Context(),
				f.owner,
				f.event,
				f.team,
				command.RequestID,
			)
			if err != nil {
				t.Fatal(err)
			}
			replayed, err := f.service.Change(t.Context(), f.owner, f.event, f.team, command)
			if err != nil {
				t.Fatal(err)
			}
			for _, receipt := range []*Receipt{recovery.Receipt, &replayed} {
				if receipt == nil || receipt.RequestID != ack.RequestID || receipt.Decision != nil {
					t.Fatal("private target body in recovered exact ACK", receipt)
				}
			}
			// The safe original request metadata survives; its original private target
			// does not become judge-visible through the newly selected public final.
			unknown, err := f.service.OriginalReceipt(
				t.Context(),
				f.owner,
				f.event,
				f.team,
				uuid.New(),
			)
			if err != nil || unknown.Receipt != nil {
				t.Fatal(unknown, err)
			}
		},
	)
}
