package checkout

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

type Entry struct {
	Directory string   `json:"directory"`
	Manifest  Manifest `json:"manifest"`
}

// List only exposes exact, locally pinned checkouts. Symlinks and unrelated
// student directories are ignored instead of being traversed.
func List(root string) ([]Entry, error) {
	root, err := WorkspaceRoot(root)
	if err != nil {
		return nil, err
	}
	courses, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0)
	for _, course := range courses {
		if !course.IsDir() || !safeSlug.MatchString(course.Name()) {
			continue
		}
		coursePath := filepath.Join(root, course.Name())
		lessons, err := os.ReadDir(coursePath)
		if err != nil {
			return nil, err
		}
		for _, lesson := range lessons {
			if !lesson.IsDir() {
				continue
			}
			lessonPath := filepath.Join(coursePath, lesson.Name())
			data, err := os.ReadFile(filepath.Join(lessonPath, ".learning", "manifest.json"))
			if err != nil {
				continue
			}
			var manifest Manifest
			if err := json.Unmarshal(data, &manifest); err != nil ||
				manifest.Schema != schemaVersion || manifest.CourseSlug != course.Name() {
				continue
			}
			entries = append(entries, Entry{Directory: lessonPath, Manifest: manifest})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Directory < entries[j].Directory
	})
	return entries, nil
}
