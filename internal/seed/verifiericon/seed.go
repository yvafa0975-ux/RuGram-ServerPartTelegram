// Package verifiericon publishes the bundled third-party verification icons in
// the operator's icon catalogue on startup.
//
// Third-party verification (core.telegram.org/api/bots/verification) renders a
// verifier's own custom-emoji icon before a peer's name. The icon must be a real
// custom emoji document the client can resolve, so the catalogue cannot be
// seeded before the documents exist: the bundled "verifiermarks" sticker set is
// imported first (see data/sticker-seed and files.Service.SeedMedia), and this
// seed then names the two blue checkmark documents it always creates.
//
// Seeding the catalogue is what makes the mark usable right after a fresh
// install: an operator can appoint a verifier against one of these entries
// without having to know a document id. Appointing the verifier itself stays an
// operator action, because it names a real bot account.
package verifiericon

import (
	"context"
	"fmt"

	"rugram/internal/domain"
)

// Catalogue is the minimal bot-verification surface the seed needs.
// *app/botverification.Service satisfies it as-is.
type Catalogue interface {
	Icons(ctx context.Context, activeOnly bool, limit int) ([]domain.VerificationIcon, error)
	UpsertIcon(ctx context.Context, icon domain.VerificationIcon) (domain.VerificationIcon, error)
}

// Default is one bundled catalogue entry.
type Default struct {
	DocumentID int64
	Name       string
}

// Defaults are the shared catalogue entries (owner_bot_id = 0, usable by any
// verifier). The document ids are the deterministic ids the bundled
// "verifiermarks" sticker seed creates: 7800000000000000025 is the checkmark,
// 7800000000000000026 is the pending/"hold" hourglass.
var Defaults = []Default{
	{DocumentID: 7800000000000000025, Name: "Verifier Check"},
	{DocumentID: 7800000000000000026, Name: "Verifier Hold"},
}

// Seed upserts the bundled icons and returns how many entries it created. It is
// idempotent: an entry is keyed by its document id, and one that already exists
// in the catalogue is left untouched.
func Seed(ctx context.Context, svc Catalogue) (int, error) {
	existing, err := svc.Icons(ctx, false, 200)
	if err != nil {
		return 0, fmt.Errorf("list verification icons: %w", err)
	}
	have := make(map[int64]bool, len(existing))
	for _, icon := range existing {
		have[icon.DocumentID] = true
	}
	created := 0
	for _, def := range Defaults {
		if have[def.DocumentID] {
			continue
		}
		if _, err := svc.UpsertIcon(ctx, domain.VerificationIcon{
			DocumentID: def.DocumentID,
			Name:       def.Name,
			Active:     true,
		}); err != nil {
			return created, fmt.Errorf("seed verification icon %d: %w", def.DocumentID, err)
		}
		created++
	}
	return created, nil
}
