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

// Enqueue durably captures a bounded dataset and target without credentials,
// HTTP, or background work. An explicit later shadow run owns the experiment
// budget and lifetime; the task's critical path performs only local capture.
func Enqueue(data Dataset, directory string, options Options) error {
	if _, err := ParseDataset(jsonBytes(data)); err != nil {
		return err
	}
	if !validTarget(options) {
		return fmt.Errorf("invalid shadow target")
	}
	if options.APIKey != "" && containsKey(data, options.APIKey) {
		return fmt.Errorf("shadow input contains the configured API key")
	}
	for _, c := range data.Cases {
		if len(requestBody(c, options.Model)) > 64<<10 {
			return fmt.Errorf("shadow request exceeds bounds")
		}
	}
	root, err := openStore(directory, true)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := installTarget(root, targetFor(options)); err != nil {
		return err
	}
	return installDataset(root, data)
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
