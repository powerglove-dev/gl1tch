package crontui

import (
	"testing"

	"github.com/8op-org/gl1tch/internal/cron"
)

func TestBuildUpdatedEntry(t *testing.T) {
	tests := []struct {
		name     string
		original cron.Entry
		editName string // if non-empty, overrides the Name field in the form
		wantArgs map[string]any
		wantDir  string
	}{
		{
			name: "rename preserves WorkingDir and Args",
			original: cron.Entry{
				Name:       "daily-report",
				Schedule:   "0 9 * * *",
				Kind:       "pipeline",
				Target:     "pipelines/report.yaml",
				Timeout:    "5m",
				WorkingDir: "/home/user/project",
				Args:       map[string]any{"env": "prod"},
			},
			editName: "weekly-report",
			wantArgs: map[string]any{"env": "prod"},
			wantDir:  "/home/user/project",
		},
		{
			name: "edit without rename preserves WorkingDir and Args",
			original: cron.Entry{
				Name:       "nightly",
				Schedule:   "0 2 * * *",
				Kind:       "pipeline",
				Target:     "pipelines/nightly.yaml",
				WorkingDir: "/srv/orcai",
				Args:       map[string]any{"dry": true},
			},
			wantArgs: map[string]any{"dry": true},
			wantDir:  "/srv/orcai",
		},
		{
			name: "new entry with no original gets zero values",
			original: cron.Entry{
				Name:     "fresh",
				Schedule: "* * * * *",
				Kind:     "pipeline",
				Target:   "pipelines/fresh.yaml",
				// Args and WorkingDir intentionally zero
			},
			wantArgs: nil,
			wantDir:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ov := newEditOverlay(tc.original)
			if tc.editName != "" {
				ov.fields[0].SetValue(tc.editName)
			}

			got := buildUpdatedEntry(ov)

			if got.WorkingDir != tc.wantDir {
				t.Errorf("WorkingDir: got %q, want %q", got.WorkingDir, tc.wantDir)
			}
			if len(got.Args) != len(tc.wantArgs) {
				t.Errorf("Args length: got %d, want %d", len(got.Args), len(tc.wantArgs))
			}
			for k, wantV := range tc.wantArgs {
				if gotV := got.Args[k]; gotV != wantV {
					t.Errorf("Args[%q]: got %v, want %v", k, gotV, wantV)
				}
			}
			if tc.editName != "" && got.Name != tc.editName {
				t.Errorf("Name after rename: got %q, want %q", got.Name, tc.editName)
			}
		})
	}
}
