package shadow

import (
	"fmt"
	"os"
	"path/filepath"
)

// ObservationDirectory separates target/policy revisions and exact captures.
func ObservationDirectory(base string, data Dataset, options Options) string {
	return filepath.Join(base, digest(jsonBytes(targetFor(options))), digest(jsonBytes(data)))
}

func installDataset(root *os.Root, data Dataset) error {
	raw, exists, err := readFile(root, "cases.json")
	if err != nil {
		return err
	}
	if exists {
		if string(raw) != string(jsonBytes(data)) {
			return fmt.Errorf("shadow capture differs from existing dataset")
		}
		return nil
	}
	return publish(root, "cases.json", jsonBytes(data))
}
