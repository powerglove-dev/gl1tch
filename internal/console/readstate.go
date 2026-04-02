package console

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// LoadReadSet loads the set of read inbox run IDs from path.
// Returns an empty map if the file doesn't exist or is corrupt.
func LoadReadSet(path string) map[int64]bool {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("deck: failed to read inbox read-state: %v", err)
		}
		return map[int64]bool{}
	}
	var ids []int64
	if err := json.Unmarshal(data, &ids); err != nil {
		log.Printf("deck: corrupt inbox read-state file, ignoring: %v", err)
		return map[int64]bool{}
	}
	set := make(map[int64]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

// SaveReadSet persists the set of read inbox run IDs to path.
func SaveReadSet(path string, ids map[int64]bool) error {
	list := make([]int64, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
