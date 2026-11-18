package giftseed

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"

	"rugram/internal/app/giftpack"
	"rugram/internal/domain"
)

// stubService is a minimal giftpack.Service that records the bundles it is
// asked to publish, so the bundled pack can be validated without a database.
type stubService struct {
	existing []domain.StarGift
	writes   []domain.StarGiftCatalogBundleWrite
}

func (s *stubService) CatalogAll(context.Context) ([]domain.StarGift, error) { return s.existing, nil }

func (s *stubService) PrepareAnimation(name string, data []byte) (domain.StarGiftAnimation, error) {
	hash := sha256.Sum256(data)
	return domain.StarGiftAnimation{
		SourceName: name,
		JSON:       data, TGS: data, SHA256: hash[:], Width: 512, Height: 512,
	}, nil
}

func (s *stubService) CreateCatalogBundle(_ context.Context, w domain.StarGiftCatalogBundleWrite) (domain.StarGiftCatalogBundleResult, error) {
	s.writes = append(s.writes, w)
	id := int64(len(s.writes) + 100)
	s.existing = append(s.existing, domain.StarGift{ID: id, Title: w.Catalog.Title})
	return domain.StarGiftCatalogBundleResult{Catalog: domain.StarGiftCatalogEntry{Gift: domain.StarGift{ID: id}}}, nil
}

// The bundled pack must parse, resolve every asset and publish all nine gifts;
// a second run must skip every title without writing anything.
func TestSeedImportsBundledClassicPack(t *testing.T) {
	svc := &stubService{}
	result, err := Seed(context.Background(), svc)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if len(result.Gifts) != 9 {
		t.Fatalf("want 9 gifts in the pack, got %d", len(result.Gifts))
	}
	for _, gift := range result.Gifts {
		if gift.Status != "created" {
			t.Fatalf("gift %q: status %q, want created", gift.Title, gift.Status)
		}
	}
	if len(svc.writes) != 9 {
		t.Fatalf("want 9 writes, got %d", len(svc.writes))
	}

	again, err := Seed(context.Background(), svc)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	for _, gift := range again.Gifts {
		if gift.Status != "skipped" {
			t.Fatalf("gift %q: status %q on repeat, want skipped", gift.Title, gift.Status)
		}
	}
	if len(svc.writes) != 9 {
		t.Fatalf("repeat run wrote %d new bundles, want 0", len(svc.writes)-9)
	}
}

// The factory pack deliberately drops the Plush Pepe gift; guard against it
// creeping back in.
func TestBundledPackHasNoPlushPepe(t *testing.T) {
	archive, err := giftpack.NewZipAssetResolver(packZip)
	if err != nil {
		t.Fatalf("open bundled pack: %v", err)
	}
	raw, err := archive.Manifest()
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifest, err := giftpack.ParseManifest(raw)
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	for _, gift := range manifest.Gifts {
		if strings.Contains(strings.ToLower(gift.Title), "plush") || strings.Contains(strings.ToLower(gift.Title), "pepe") {
			t.Fatalf("bundled pack contains a removed gift: %q", gift.Title)
		}
	}
}
