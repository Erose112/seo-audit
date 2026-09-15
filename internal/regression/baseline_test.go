package regression

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Erose112/seo-audit/internal/report"
)

func TestLoadBaselineEmptyPath(t *testing.T) {
	t.Parallel()
	rep, err := LoadBaseline("")
	if err != nil {
		t.Fatalf("LoadBaseline(\"\"): %v", err)
	}
	if rep.SchemaVersion != 0 {
		t.Errorf("SchemaVersion = %d, want 0", rep.SchemaVersion)
	}
}

func TestLoadBaselineMissingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rep, err := LoadBaseline(filepath.Join(dir, "missing.json"))
	if err != nil {
		t.Fatalf("LoadBaseline(missing): %v", err)
	}
	if rep.SchemaVersion != 0 {
		t.Errorf("SchemaVersion = %d, want 0", rep.SchemaVersion)
	}
}

func TestLoadBaselineMalformedJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadBaseline(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestLoadBaselineSchemaMismatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "old.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":99,"score":80}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadBaseline(path)
	if err == nil {
		t.Fatal("expected schema mismatch error")
	}
}

func TestSaveBaselineRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")
	want := report.Report{
		SchemaVersion: report.SchemaVersion,
		URL:           "https://example.com/",
		Timestamp:     time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
		Score:         91,
	}
	if err := SaveBaseline(path, want); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Error("leftover .tmp file after successful save")
	}
	got, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	if got.Score != want.Score || got.URL != want.URL {
		t.Errorf("round-trip = %+v, want score %d url %q", got, want.Score, want.URL)
	}
}

func TestLoadReportMissingIsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := LoadReport(filepath.Join(dir, "missing.json"))
	if err == nil {
		t.Fatal("expected error for missing file in LoadReport")
	}
}
