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
	dir := "/Users/stokes/Projects/observability-robots/.glitch/workflows"
	a, _ := filepath.Glob(filepath.Join(dir, "*.workflow.yaml"))
	b, _ := filepath.Glob(filepath.Join(dir, "*.pipeline.yaml"))
	files := append(a, b...)
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
		fmt.Printf("\n%d pipelines failed validation\n", failed)
		os.Exit(1)
	}
	fmt.Println("\nAll pipelines valid")
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
