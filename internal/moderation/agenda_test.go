package moderation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestAgendaMetadataDoesNotJoinPrivatePanelOrExposeAssessment(t *testing.T) {
	f, s, h := finalContext(t, true)
	outsider := testdb.Person(t, f.pool)
	f.exec(`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'organiser')`, f.event, outsider.ID)
	read := func(actor uuid.UUID) (Agenda, error) {
		var out Agenda
		err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			var e error
			out, e = s.AgendaInTransaction(t.Context(), tx, actor, f.event, f.team, uuid.Nil, 10)
			return e
		})
		return out, err
	}
	page, err := read(outsider.ID)
	if err != nil || len(page.Items) != 1 || page.Items[0].CanOpen || page.Items[0].CanManage || page.Items[0].CanReconfigure || !page.Items[0].SecondComplete {
		t.Fatal("organiser coordination incorrectly grants private panel", page, err)
	}
	raw, _ := json.Marshal(page)
	for _, private := range []string{`"scores":`, `"cards":`, `"evidence":`, `"proposals":`, `"reason":`, "PRIVATE-JUDGE-CANARY"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private payload leaked", private)
		}
	}
	if _, err = s.Panel(t.Context(), outsider.ID, f.event, f.team, h.Definition.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("metadata reader joined panel", err)
	}
	page, err = read(f.owner.ID)
	if err != nil || !page.Items[0].CanOpen || !page.Items[0].CanManage || page.Items[0].CanonicalSigned {
		t.Fatal("current named moderator missing native operation", page, err)
	}
	h = f.change(s, h, f.owner.ID, Command{Action: "sign_off", Reason: "Exact current final proof"})
	page, err = read(outsider.ID)
	if err != nil || !page.Items[0].CanonicalSigned || page.Items[0].CanOpen {
		t.Fatal("canonical fact absent or panel disclosed", page, err)
	}
	f.exec(`UPDATE events SET writes_allowed=false,stage='archived' WHERE id=$1`, f.event)
	page, err = read(f.owner.ID)
	if err != nil || page.Items[0].CanManage || !page.Items[0].CanonicalSigned {
		t.Fatal("archive fabricated writes or lost retained proof", page, err)
	}
	f.exec(`UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2`, f.event, outsider.ID)
	if _, err = read(outsider.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("revoked organiser retained metadata", err)
	}
	if _, err = read(f.member.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("competitor gained private coordination", err)
	}
}
