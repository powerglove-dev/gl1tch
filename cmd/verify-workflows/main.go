// verify-workflows is a static checker for workflow YAML files.
//
// Usage:
//
//	verify-workflows [dir]
//
// If [dir] is omitted, the current directory's .glitch/workflows/ is scanned.
// All .workflow.yaml files in the directory are loaded and walked for
// `{{ steps.X.Y }}` refs that point at unknown step IDs or undeclared outputs.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

var stepRefPattern = regexp.MustCompile(`\{\{\s*steps\.([^.}\s]+)\.([^}\s]+)\s*\}\}`)

func main() {
	dir := defaultDir()
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	files, _ := filepath.Glob(filepath.Join(dir, "*.workflow.yaml"))
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "no .workflow.yaml files found in %s\n", dir)
		os.Exit(1)
	}

	failed := 0
	for _, f := range files {
		fp, err := os.Open(f)
		if err != nil {
			fmt.Printf("[ERR] %s: open: %v\n", filepath.Base(f), err)
			failed++
			continue
		}
		p, err := pipeline.Load(fp)
		fp.Close()
		if err != nil {
			fmt.Printf("[ERR] %s: load: %v\n", filepath.Base(f), err)
			failed++
			continue
		}

		// Build map of declared outputs per step
		declared := map[string]map[string]bool{}
		for _, s := range p.Steps {
			declared[s.ID] = map[string]bool{}
			for k := range s.Outputs {
				declared[s.ID][k] = true
			}
		}

		ok := true
		// Walk all step prompts and find {{ steps.X.Y }} refs
		for _, s := range p.Steps {
			text := s.Prompt + " " + s.Input
			matches := stepRefPattern.FindAllStringSubmatch(text, -1)
			for _, m := range matches {
				stepID, key := m[1], m[2]
				if declared[stepID] == nil {
					fmt.Printf("[ERR] %s: step %q references unknown step %q\n",
						filepath.Base(f), s.ID, stepID)
					ok = false
					continue
				}
				if !declared[stepID][key] {
					fmt.Printf("[ERR] %s: step %q references {{ steps.%s.%s }} but %s does not declare outputs.%s (declared: %s)\n",
						filepath.Base(f), s.ID, stepID, key, stepID, key, mapKeys(declared[stepID]))
					ok = false
				}
			}
		}
		if ok {
			fmt.Printf("[OK]  %s (%d steps)\n", filepath.Base(f), len(p.Steps))
		} else {
			failed++
		}
	}
	if failed > 0 {
		fmt.Printf("\n%d workflows failed validation\n", failed)
		os.Exit(1)
	}
	fmt.Println("\nAll workflows valid")
}

func defaultDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ".glitch/workflows"
	}
	return filepath.Join(cwd, ".glitch", "workflows")
}

func mapKeys(m map[string]bool) string {
	if len(m) == 0 {
		return "none"
	}
	keys := []string{}
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ",")
}
