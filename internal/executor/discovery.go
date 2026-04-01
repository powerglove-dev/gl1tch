package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadWrappers scans dir for *.yaml sidecar files and returns the resulting executors.
// Files that fail to parse are skipped; their errors are collected and returned.
// If dir does not exist, an empty slice and nil errors are returned.
func LoadWrappers(dir string) ([]Executor, []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []error{fmt.Errorf("load wrappers: read dir %s: %w", dir, err)}
	}

	var executors []Executor
	var errs []error
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		p, err := NewCliAdapterFromSidecar(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		executors = append(executors, p)
	}
	return executors, errs
}
