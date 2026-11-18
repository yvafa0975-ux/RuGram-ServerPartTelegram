package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeDriftFiles drops a pair of .env / .env.example into a temp dir and returns
// their paths. An omitted argument means the file does not exist.
func writeDriftFiles(t *testing.T, example, env string) (cfgPath, examplePath string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath = filepath.Join(dir, ".env")
	examplePath = cfgPath + ".example"
	if example != "" {
		if err := os.WriteFile(examplePath, []byte(example), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if env != "" {
		if err := os.WriteFile(cfgPath, []byte(env), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return cfgPath, examplePath
}

func TestDetectEnvDriftSplitsMissingAndUnknown(t *testing.T) {
	cfgPath, examplePath := writeDriftFiles(t,
		"## Group -- d\nRUGRAM_ALPHA=1\n# RUGRAM_OPTIONAL=\nRUGRAM_BETA=2\n",
		"RUGRAM_BETA=custom\nRUGRAM_TYPO_KEY=x\n")

	drift, err := envDriftAt(cfgPath, examplePath)
	if err != nil {
		t.Fatalf("envDriftAt: %v", err)
	}
	// ALPHA and the commented-out OPTIONAL are both declared in the template and unset
	// locally -> Missing, preserving template order.
	if want := []string{"RUGRAM_ALPHA", "RUGRAM_OPTIONAL"}; !reflect.DeepEqual(drift.Missing, want) {
		t.Errorf("Missing = %v, want %v", drift.Missing, want)
	}
	// BETA is in both; TYPO_KEY is only in .env -> Unknown (a typo signal).
	if want := []string{"RUGRAM_TYPO_KEY"}; !reflect.DeepEqual(drift.Unknown, want) {
		t.Errorf("Unknown = %v, want %v", drift.Unknown, want)
	}
}

func TestDetectEnvDriftProcessEnvCountsAsSet(t *testing.T) {
	cfgPath, examplePath := writeDriftFiles(t,
		"RUGRAM_ALPHA=\nRUGRAM_BETA=\n",
		"")
	// The process environment takes precedence over the file: being set there means
	// not missing, even if .env lacks it.
	t.Setenv("RUGRAM_ALPHA", "from-process")

	drift, err := envDriftAt(cfgPath, examplePath)
	if err != nil {
		t.Fatalf("envDriftAt: %v", err)
	}
	if want := []string{"RUGRAM_BETA"}; !reflect.DeepEqual(drift.Missing, want) {
		t.Errorf("Missing = %v, want %v", drift.Missing, want)
	}
}

func TestDetectEnvDriftMissingTemplateIsNotAnError(t *testing.T) {
	// Production images carry no .env.example: it must be silently skipped, not fail
	// at startup.
	cfgPath, examplePath := writeDriftFiles(t, "", "RUGRAM_ALPHA=1\n")

	drift, err := envDriftAt(cfgPath, examplePath)
	if err != nil {
		t.Fatalf("envDriftAt: %v", err)
	}
	if len(drift.Missing) != 0 || len(drift.Unknown) != 0 {
		t.Errorf("drift = %+v, want zero value when template is absent", drift)
	}
}

func TestDetectEnvDriftWithoutConfigFileListsTemplateDefaults(t *testing.T) {
	// With no .env (config entirely from the process environment/defaults), every key
	// in the template lands in Missing and the caller presents them at INFO -- exactly
	// the visibility a fresh install / pure environment-variable deployment needs.
	cfgPath, examplePath := writeDriftFiles(t,
		"RUGRAM_ALPHA=1\nRUGRAM_BETA=2\n",
		"")

	drift, err := envDriftAt(cfgPath, examplePath)
	if err != nil {
		t.Fatalf("envDriftAt: %v", err)
	}
	if want := []string{"RUGRAM_ALPHA", "RUGRAM_BETA"}; !reflect.DeepEqual(drift.Missing, want) {
		t.Errorf("Missing = %v, want %v", drift.Missing, want)
	}
	if len(drift.Unknown) != 0 {
		t.Errorf("Unknown = %v, want none", drift.Unknown)
	}
}

func TestDetectEnvDriftUnknownKeysAreSorted(t *testing.T) {
	// .env is read into a map with random iteration order, so the report must be
	// stable (both logs and assertions depend on it).
	cfgPath, examplePath := writeDriftFiles(t,
		"RUGRAM_KEEP=1\n",
		"RUGRAM_ZULU=1\nRUGRAM_ALFA=1\nRUGRAM_KEEP=2\n")

	drift, err := envDriftAt(cfgPath, examplePath)
	if err != nil {
		t.Fatalf("envDriftAt: %v", err)
	}
	if want := []string{"RUGRAM_ALFA", "RUGRAM_ZULU"}; !reflect.DeepEqual(drift.Unknown, want) {
		t.Errorf("Unknown = %v, want %v", drift.Unknown, want)
	}
}

func TestDetectEnvDriftIgnoresNonFieldLines(t *testing.T) {
	// Group headers, explanatory comments and non-RUGRAM_ lines in the template must
	// not be treated as field keys.
	cfgPath, examplePath := writeDriftFiles(t,
		"## Server & Network -- Where the server listens\n"+
			"# это просто комментарий без ключа\n"+
			"# =====\n"+
			"POSTGRES_PASSWORD=irrelevant\n"+
			"RUGRAM_LISTEN=127.0.0.1:2398\n",
		"RUGRAM_LISTEN=127.0.0.1:2398\n")

	drift, err := envDriftAt(cfgPath, examplePath)
	if err != nil {
		t.Fatalf("envDriftAt: %v", err)
	}
	if len(drift.Missing) != 0 || len(drift.Unknown) != 0 {
		t.Errorf("drift = %+v, want no drift for pure documentation lines", drift)
	}
}

func TestDetectEnvDriftAcceptsConfigKeySyntaxWithoutLoggingValues(t *testing.T) {
	cfgPath, examplePath := writeDriftFiles(t,
		"export RUGRAM_ALPHA = a-secret\n# export RUGRAM_OPTIONAL = example-secret\nRUGRAM_BETA=template-secret\n",
		"export RUGRAM_ALPHA = actual-secret\nRUGRAM_TYPO=another-secret\n")
	drift, err := envDriftAt(cfgPath, examplePath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(drift.Missing, []string{"RUGRAM_OPTIONAL", "RUGRAM_BETA"}) ||
		!reflect.DeepEqual(drift.Unknown, []string{"RUGRAM_TYPO"}) {
		t.Fatalf("unexpected drift: %+v", drift)
	}
}

// TestRepoEnvExampleParses verifies the real .env.example at the repo root parses
// fully for the drift comparison: group headers, explanatory comments and placeholder
// lines must not be taken as fields, and a documentation-scale batch of field keys
// must be parsed out. (The reverse direction, "template has it but no code reads it",
// involves dynamic keys and out-of-tree binaries and is not guarded here.)
func TestRepoEnvExampleParses(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	keys, err := readExampleKeys(filepath.Join(repoRoot, ".env.example"))
	if err != nil {
		t.Fatalf("readExampleKeys: %v", err)
	}
	if len(keys) < 100 {
		t.Fatalf("parsed %d keys from .env.example, want the documented hundreds", len(keys))
	}
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] {
			t.Errorf("duplicate key %s in .env.example", k)
		}
		seen[k] = true
	}
}
