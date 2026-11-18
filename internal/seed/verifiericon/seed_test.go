package verifiericon

import (
	"context"
	"testing"

	"rugram/internal/domain"
)

type stubCatalogue struct {
	icons   []domain.VerificationIcon
	upserts int
}

func (s *stubCatalogue) Icons(context.Context, bool, int) ([]domain.VerificationIcon, error) {
	return s.icons, nil
}

func (s *stubCatalogue) UpsertIcon(_ context.Context, icon domain.VerificationIcon) (domain.VerificationIcon, error) {
	s.upserts++
	s.icons = append(s.icons, icon)
	return icon, nil
}

func TestSeedAddsMissingIconsAndIsIdempotent(t *testing.T) {
	svc := &stubCatalogue{}
	created, err := Seed(context.Background(), svc)
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if created != len(Defaults) || svc.upserts != len(Defaults) {
		t.Fatalf("first seed: created=%d upserts=%d, want %d", created, svc.upserts, len(Defaults))
	}

	created, err = Seed(context.Background(), svc)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if created != 0 || svc.upserts != len(Defaults) {
		t.Fatalf("second seed: created=%d upserts=%d, want 0 and %d", created, svc.upserts, len(Defaults))
	}
}
