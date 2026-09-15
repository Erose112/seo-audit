package regression

import (
	"errors"
	"fmt"
	"os"

	json "encoding/json/v2"

	"github.com/Erose112/seo-audit/internal/report"
)

// LoadBaseline returns a zero-value report when path is empty or the file is
// missing (first run). Corrupt or schema-mismatched files are hard errors.
func LoadBaseline(path string) (report.Report, error) {
	if path == "" {
		return report.Report{}, nil
	}
	return loadReport(path, true)
}

// LoadReport requires the file to exist and parse as a report.
func LoadReport(path string) (report.Report, error) {
	return loadReport(path, false)
}

func loadReport(path string, missingIsOK bool) (report.Report, error) {
	f, err := os.Open(path)
	if err != nil {
		if missingIsOK && errors.Is(err, os.ErrNotExist) {
			return report.Report{}, nil
		}
		return report.Report{}, fmt.Errorf("regression: open %s: %w", path, err)
	}
	defer f.Close()

	var rep report.Report
	if err := json.UnmarshalRead(f, &rep); err != nil {
		return report.Report{}, fmt.Errorf("regression: parse %s: %w", path, err)
	}
	if rep.SchemaVersion != 0 && rep.SchemaVersion != report.SchemaVersion {
		return report.Report{}, fmt.Errorf(
			"regression: %s has schema_version %d, want %d",
			path, rep.SchemaVersion, report.SchemaVersion,
		)
	}
	return rep, nil
}

// SaveBaseline atomically writes the report to path via a temp file and rename.
func SaveBaseline(path string, r report.Report) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("regression: create %s: %w", tmp, err)
	}
	if err := r.WriteJSON(f); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("regression: write %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("regression: close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("regression: rename %s: %w", path, err)
	}
	return nil
}
