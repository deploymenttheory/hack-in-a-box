package presentation

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestDisplayAssetsAreCurrentApprovedEventBrandingOnly(t *testing.T) {
	f := newFixture(t)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	asset := templates.Asset{
		ID:        uuid.New(),
		Kind:      "logo",
		MediaType: "image/png",
		Content:   encoded.Bytes(),
	}
	defs := templates.EmptyDefinitions()
	defs.Branding.Assets = []templates.Asset{asset}
	defs.Messages = []templates.Message{
		{ID: uuid.New(), Name: "PRIVATE MESSAGE CANARY", Body: "PRIVATE MESSAGE BODY"},
	}
	save := func(id uuid.UUID, definitions templates.Definitions) {
		t.Helper()
		raw, err := json.Marshal(definitions)
		if err != nil {
			t.Fatal(err)
		}
		f.exec(
			"INSERT INTO event_definitions(event_id,definitions) VALUES($1,$2) ON CONFLICT(event_id) DO UPDATE SET definitions=EXCLUDED.definitions",
			id,
			raw,
		)
	}
	save(f.id, defs)
	paired := f.pair(f.mc.ID)
	snapshot := f.snapshot(paired.Token)
	digest := sha256.Sum256(asset.Content)
	if len(snapshot.Assets) != 1 || snapshot.Assets[0].ID != asset.ID ||
		snapshot.Assets[0].Digest != fmt.Sprintf("%x", digest) {
		t.Fatalf("asset metadata: %#v", snapshot.Assets)
	}
	raw, _ := json.Marshal(snapshot)
	if bytes.Contains(raw, []byte("PRIVATE")) || bytes.Contains(raw, []byte("iVBOR")) {
		t.Fatal("snapshot leaked definitions or image bytes")
	}
	got, err := f.service.Asset(t.Context(), f.id, paired.Token, asset.ID)
	if err != nil || !bytes.Equal(got.Content, asset.Content) {
		t.Fatalf("approved raster: %v", err)
	}
	foreign := testdb.Event(t, f.pool, f.owner.ID)
	other := asset
	other.ID = uuid.New()
	otherDefs := templates.EmptyDefinitions()
	otherDefs.Branding.Assets = []templates.Asset{other}
	save(foreign, otherDefs)
	if _, err = f.service.Asset(
		t.Context(),
		f.id,
		paired.Token,
		other.ID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatalf("foreign asset became local: %v", err)
	}
	if _, err = f.service.Asset(
		t.Context(),
		foreign,
		paired.Token,
		other.ID,
	); !errors.Is(
		err,
		access.ErrUnauthenticated,
	) {
		t.Fatalf("cross-event token: %v", err)
	}
	defs.Branding.Assets = nil
	save(f.id, defs)
	replacement := f.snapshot(paired.Token)
	if replacement.Sequence != snapshot.Sequence || len(replacement.Assets) != 0 {
		t.Fatal("branding removal must project without changing cue sequence")
	}
	if _, err = f.service.Asset(
		t.Context(),
		f.id,
		paired.Token,
		asset.ID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatalf("removed asset survived: %v", err)
	}
	// Reintroducing corrupt legacy bytes must not turn this into a generic file reader.
	asset.Content = []byte("<svg onload=alert(1)>")
	defs.Branding.Assets = []templates.Asset{asset}
	save(f.id, defs)
	if len(f.snapshot(paired.Token).Assets) != 0 {
		t.Fatal("unvalidated executable asset projected")
	}
	if _, err = f.service.Asset(
		t.Context(),
		f.id,
		paired.Token,
		asset.ID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatalf("invalid raster survived: %v", err)
	}
	save(f.id, otherDefs)
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mc'",
		f.id,
		f.mc.ID,
	)
	if _, err = f.service.Asset(
		t.Context(),
		f.id,
		paired.Token,
		other.ID,
	); !errors.Is(
		err,
		access.ErrUnauthenticated,
	) {
		t.Fatalf("revoked issuer asset: %v", err)
	}
}
