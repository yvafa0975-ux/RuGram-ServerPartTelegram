// Package giftseed bundles the classic Telegram star-gift pack into the server
// binary and imports it into the durable gift catalogue on startup.
//
// The nine classic gifts (Teddy Bear, Love Heart, Gift Box, Red Rose, Birthday
// Cake, Champagne Bottle, Trophy, Diamond Ring, Gem) are what a fresh
// deployment should offer with no operator action, so the pack travels with the
// binary instead of an on-disk seed directory that a checkout may not have.
//
// Import is idempotent at the gift-title level (giftpack.Import skips a title
// that already exists), so calling Seed on every boot only ever publishes the
// gifts that are missing.
package giftseed

import (
	"context"
	_ "embed"
	"fmt"

	"rugram/internal/app/giftpack"
)

// packZip is the bundled classic gift pack: pack.json plus the nine .tgs
// bodies. It is embedded so the seed never depends on the working directory or
// a bind mount.
//
//go:embed classic-gifts-pack.zip
var packZip []byte

// actor and commandID label the catalogue revisions the seed creates. They are
// stable on purpose: a title that already exists is skipped before any write,
// so a repeated boot never reuses the command id for a second revision.
const (
	actor     = "gift-seed"
	commandID = "seed-classic-gifts"
)

// Seed imports the bundled pack into the catalogue and reports the per-gift
// outcome. A gift whose title already exists is reported as "skipped".
func Seed(ctx context.Context, svc giftpack.Service) (giftpack.ImportResult, error) {
	archive, err := giftpack.NewZipAssetResolver(packZip)
	if err != nil {
		return giftpack.ImportResult{}, fmt.Errorf("open bundled gift pack: %w", err)
	}
	manifestBytes, err := archive.Manifest()
	if err != nil {
		return giftpack.ImportResult{}, fmt.Errorf("read bundled gift pack manifest: %w", err)
	}
	manifest, err := giftpack.ParseManifest(manifestBytes)
	if err != nil {
		return giftpack.ImportResult{}, fmt.Errorf("parse bundled gift pack manifest: %w", err)
	}
	result, err := giftpack.Import(ctx, svc, manifest, archive, giftpack.ImportOptions{
		Actor:     actor,
		CommandID: commandID,
	})
	if err != nil {
		return result, fmt.Errorf("import bundled gift pack: %w", err)
	}
	return result, nil
}
