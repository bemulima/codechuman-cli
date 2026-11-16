package checkout

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"
)

const materialCacheSchema = "codechuman.ide-material-cache.v1"

type materialCacheFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type materialCacheManifest struct {
	Schema         string              `json:"schema"`
	TaskInstanceID string              `json:"task_instance_id"`
	CourseID       string              `json:"course_id"`
	LessonID       string              `json:"lesson_id"`
	LessonRevision string              `json:"lesson_revision"`
	Files          []materialCacheFile `json:"files"`
}

type MaterialCacheResult struct {
	Directory string `json:"directory"`
	Reused    bool   `json:"reused"`
}

// CacheRemoteMaterials installs an already-authorized public pack in one
// revision-specific directory. The directory appears only after all files and
// its manifest have been written and checked. Existing cache data is immutable.
func CacheRemoteMaterials(checkoutDir string, spec Spec, materials []Material) (MaterialCacheResult, error) {
	if err := spec.Validate(); err != nil {
		return MaterialCacheResult{}, fmt.Errorf("invalid pinned checkout: %w", err)
	}
	for name, value := range map[string]string{
		"task_instance_id": spec.TaskInstanceID, "course_id": spec.CourseID,
		"lesson_id": spec.LessonID, "lesson_revision": spec.LessonRevision,
		"sandbox_id": spec.SandboxID,
	} {
		if !validMaterialIdentity(value) {
			return MaterialCacheResult{}, fmt.Errorf("invalid pinned %s", name)
		}
	}
	if len(materials) == 0 {
		return MaterialCacheResult{}, errors.New("materials pack is empty")
	}
	candidate := spec
	candidate.Materials = materials
	if err := candidate.Validate(); err != nil {
		return MaterialCacheResult{}, fmt.Errorf("unsafe materials pack: %w", err)
	}
	hasLesson := false
	for _, file := range materials {
		if !utf8.ValidString(file.Body) {
			return MaterialCacheResult{}, errors.New("material is not valid UTF-8")
		}
		if file.Path == "lesson.md" {
			hasLesson = true
		}
		if file.Path == "quiz.json" {
			if err := validatePublicQuiz(file.Body); err != nil {
				return MaterialCacheResult{}, err
			}
		}
	}
	if !hasLesson {
		return MaterialCacheResult{}, errors.New("materials pack requires a pinned lesson")
	}
	manifest := newMaterialCacheManifest(spec, materials)
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return MaterialCacheResult{}, err
	}
	manifestBytes = append(manifestBytes, '\n')

	checkoutInfo, err := os.Lstat(checkoutDir)
	if err != nil || !checkoutInfo.IsDir() || checkoutInfo.Mode()&os.ModeSymlink != 0 {
		return MaterialCacheResult{}, ErrTargetOccupied
	}
	learningDir := filepath.Join(checkoutDir, ".learning")
	learningInfo, err := os.Lstat(learningDir)
	if err != nil || !learningInfo.IsDir() || learningInfo.Mode()&os.ModeSymlink != 0 {
		return MaterialCacheResult{}, ErrTargetOccupied
	}
	if err := requireMatchingCheckoutManifest(learningDir, spec); err != nil {
		return MaterialCacheResult{}, err
	}
	materialsRoot := filepath.Join(learningDir, "materials")
	if err := ensureRealDirectory(materialsRoot, 0o700); err != nil {
		return MaterialCacheResult{}, err
	}
	destination := filepath.Join(materialsRoot, spec.LessonRevision)
	if _, err := os.Lstat(destination); err == nil {
		if err := verifyMaterialCache(destination, manifest, manifestBytes, materials); err != nil {
			return MaterialCacheResult{}, ErrRevisionConflict
		}
		return MaterialCacheResult{Directory: destination, Reused: true}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return MaterialCacheResult{}, err
	}

	staging, err := os.MkdirTemp(materialsRoot, ".staging-")
	if err != nil {
		return MaterialCacheResult{}, err
	}
	defer os.RemoveAll(staging) // Unique directory created by this invocation.
	for _, asset := range materials {
		filePath := filepath.Join(staging, filepath.FromSlash(asset.Path))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
			return MaterialCacheResult{}, err
		}
		file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return MaterialCacheResult{}, err
		}
		_, writeErr := file.WriteString(asset.Body)
		syncErr := file.Sync()
		closeErr := file.Close()
		if writeErr != nil {
			return MaterialCacheResult{}, writeErr
		}
		if syncErr != nil {
			return MaterialCacheResult{}, syncErr
		}
		if closeErr != nil {
			return MaterialCacheResult{}, closeErr
		}
	}
	if err := os.WriteFile(filepath.Join(staging, "manifest.json"), manifestBytes, 0o600); err != nil {
		return MaterialCacheResult{}, err
	}
	if err := verifyMaterialCache(staging, manifest, manifestBytes, materials); err != nil {
		return MaterialCacheResult{}, err
	}
	if err := os.Rename(staging, destination); err != nil {
		if _, statErr := os.Lstat(destination); statErr == nil {
			if verifyErr := verifyMaterialCache(destination, manifest, manifestBytes, materials); verifyErr == nil {
				return MaterialCacheResult{Directory: destination, Reused: true}, nil
			}
			return MaterialCacheResult{}, ErrRevisionConflict
		}
		return MaterialCacheResult{}, err
	}
	return MaterialCacheResult{Directory: destination}, nil
}

func validMaterialIdentity(value string) bool {
	return canonicalTaskID.MatchString(value) && value != "00000000-0000-0000-0000-000000000000"
}

func newMaterialCacheManifest(spec Spec, materials []Material) materialCacheManifest {
	files := make([]materialCacheFile, 0, len(materials))
	for _, item := range materials {
		sum := sha256.Sum256([]byte(item.Body))
		files = append(files, materialCacheFile{Path: item.Path, SHA256: fmt.Sprintf("sha256:%x", sum)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return materialCacheManifest{
		Schema: materialCacheSchema, TaskInstanceID: spec.TaskInstanceID,
		CourseID: spec.CourseID, LessonID: spec.LessonID,
		LessonRevision: spec.LessonRevision, Files: files,
	}
}

func ensureRealDirectory(path string, mode os.FileMode) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(path, mode); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrTargetOccupied
	}
	return nil
}

func requireMatchingCheckoutManifest(learningDir string, spec Spec) error {
	manifestPath := filepath.Join(learningDir, "manifest.json")
	info, err := os.Lstat(manifestPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 16*1024 {
		return ErrTargetOccupied
	}
	file, err := os.Open(manifestPath)
	if err != nil {
		return ErrTargetOccupied
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 16*1024))
	decoder.DisallowUnknownFields()
	var existing Manifest
	if err := decoder.Decode(&existing); err != nil || existing != spec.Manifest() {
		return ErrRevisionConflict
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return ErrRevisionConflict
	}
	return nil
}

func verifyMaterialCache(directory string, manifest materialCacheManifest, manifestBytes []byte, materials []Material) error {
	rootInfo, err := os.Lstat(directory)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return ErrRevisionConflict
	}
	manifestPath := filepath.Join(directory, "manifest.json")
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil || !manifestInfo.Mode().IsRegular() || manifestInfo.Mode()&os.ModeSymlink != 0 || manifestInfo.Size() != int64(len(manifestBytes)) {
		return ErrRevisionConflict
	}
	actualManifest, err := os.ReadFile(manifestPath)
	if err != nil || !bytes.Equal(actualManifest, manifestBytes) {
		return ErrRevisionConflict
	}

	expected := make(map[string]Material, len(materials))
	allowedDirs := map[string]bool{".": true}
	for _, item := range materials {
		expected[filepath.FromSlash(item.Path)] = item
		for parent := filepath.Dir(filepath.FromSlash(item.Path)); parent != "."; parent = filepath.Dir(parent) {
			allowedDirs[parent] = true
		}
	}
	seen := map[string]bool{"manifest.json": true}
	err = filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == directory {
			return nil
		}
		relative, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return ErrRevisionConflict
		}
		if entry.IsDir() {
			if !allowedDirs[relative] {
				return ErrRevisionConflict
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return ErrRevisionConflict
		}
		if relative == "manifest.json" {
			return nil
		}
		item, ok := expected[relative]
		if !ok || seen[relative] {
			return ErrRevisionConflict
		}
		info, err := entry.Info()
		if err != nil || info.Size() > 512*1024 {
			return ErrRevisionConflict
		}
		body, err := os.ReadFile(path)
		if err != nil || string(body) != item.Body {
			return ErrRevisionConflict
		}
		seen[relative] = true
		return nil
	})
	if err != nil || len(seen) != len(expected)+1 {
		return ErrRevisionConflict
	}
	return nil
}
