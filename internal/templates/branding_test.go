package templates

import (
	"errors"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestBrandValidation(t *testing.T) {
	if err := validateBranding(EmptyDefinitions().Branding); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Branding){
		"unreadable text":    func(b *Branding) { b.Foreground = b.Background },
		"unreadable control": func(b *Branding) { b.Primary = b.Background },
		"CSS injection":      func(b *Branding) { b.Accent = "red; background:url(x)" },
		"SVG": func(b *Branding) {
			b.Assets[0].Content = []byte("<svg><script>alert(1)</script></svg>")
			b.Assets[0].MediaType = "image/svg+xml"
		},
		"wrong type":      func(b *Branding) { b.Assets[0].MediaType = "image/jpeg" },
		"truncated image": func(b *Branding) { b.Assets[0].Content = b.Assets[0].Content[:len(b.Assets[0].Content)-12] },
		"oversize":        func(b *Branding) { b.Assets[0].Content = make([]byte, 512*1024+1) },
		"duplicate kind":  func(b *Branding) { b.Assets = append(b.Assets, b.Assets[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			b := fixture().Definitions.Branding
			mutate(&b)
			if validateBranding(b) == nil {
				t.Fatal("unsafe brand accepted")
			}
		})
	}
}

func TestBrandIsolationRevisionAndPrivateDefinitions(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner := testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	otherOwner := testdb.Person(t, pool)
	other := testdb.Event(t, pool, otherOwner.ID)
	s := Service{Pool: pool}
	defs := EmptyDefinitions()
	defs.Forms = fixture().Definitions.Forms
	defs.RoleBriefs = fixture().Definitions.RoleBriefs
	row, _ := q.Event(ctx, id)
	if _, err := s.SaveDefinitions(ctx, owner.ID, id, row.Revision, defs); err != nil {
		t.Fatal(err)
	}
	read, err := s.ReadBrand(ctx, owner.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	brand := read.Branding
	brand.Primary = "#233c78"
	saved, err := s.SaveBrand(ctx, owner.ID, id, read.Revision, brand)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != read.Revision+1 {
		t.Fatal("revision not advanced")
	}
	if _, err := s.SaveBrand(
		ctx,
		owner.ID,
		id,
		read.Revision,
		brand,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("stale save accepted")
	}
	if _, err := s.SaveBrand(ctx, owner.ID, other, 1, brand); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("cross-event write accepted")
	}
	if _, err := s.ReadBrand(ctx, otherOwner.ID, id); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("cross-event read accepted")
	}
	snapshot, err := s.Snapshot(ctx, owner.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Definitions.RoleBriefs[0].Body != defs.RoleBriefs[0].Body ||
		len(snapshot.Definitions.Forms) != 1 {
		t.Fatal("branding overwrote private definitions")
	}
	unchanged, err := s.ReadBrand(ctx, otherOwner.ID, other)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Branding.Primary != EmptyDefinitions().Branding.Primary {
		t.Fatal("other event branding changed")
	}
	viewer := testdb.Person(t, pool)
	if err := q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{EventID: id, PersonID: viewer.ID, Role: "mentor", Active: true},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadBrand(ctx, viewer.ID, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Snapshot(ctx, viewer.ID, id); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("private definitions visible to branding reader")
	}
	if err := q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{EventID: id, PersonID: viewer.ID, Role: "mentor", Active: false},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadBrand(ctx, viewer.ID, id); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("revoked membership retained branding access")
	}
}
