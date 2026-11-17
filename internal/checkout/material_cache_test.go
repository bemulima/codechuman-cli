package checkout

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createMaterialCheckout(t *testing.T) (string, Spec) {
	t.Helper()
	root := t.TempDir()
	spec := materialSpec("https://gateway.example")
	result, err := Checkout(context.Background(), root, spec, &stubCloner{})
	if err != nil {
		t.Fatalf("create checkout: %v", err)
	}
	return result.Directory, spec
}

func TestCacheRemoteMaterialsWritesRevisionScopedCacheAndPreservesWork(t *testing.T) {
	directory, spec := createMaterialCheckout(t)
	materials := sampleMaterialPack().Files
	files := make([]Material, 0, len(materials))
	for _, item := range materials {
		files = append(files, Material{Path: item.Path, Body: item.Body})
	}

	first, err := CacheRemoteMaterials(directory, spec, files)
	if err != nil {
		t.Fatalf("cache authorized public materials: %v", err)
	}
	wantDir := filepath.Join(directory, ".learning", "materials", spec.LessonRevision)
	if first.Directory != wantDir || first.Reused {
		t.Fatalf("unexpected first cache result: %+v", first)
	}
	lessonPath := filepath.Join(wantDir, "lesson.md")
	lesson, err := os.ReadFile(lessonPath)
	if err != nil || string(lesson) != "# Lesson 3\n" {
		t.Fatalf("cached lesson mismatch: %q, %v", lesson, err)
	}
	info, err := os.Stat(lessonPath)
	if err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("material permissions are too broad: %v, %v", info, err)
	}
	appPath := filepath.Join(directory, "app.js")
	if err := os.WriteFile(appPath, []byte("// student's unfinished work\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := CacheRemoteMaterials(directory, spec, files)
	if err != nil || !second.Reused {
		t.Fatalf("expected exact immutable cache reuse: %+v, %v", second, err)
	}
	app, err := os.ReadFile(appPath)
	if err != nil || string(app) != "// student's unfinished work\n" {
		t.Fatalf("local source was changed: %q, %v", app, err)
	}
}

func TestCacheRemoteMaterialsRefusesToOverwriteModifiedCache(t *testing.T) {
	directory, spec := createMaterialCheckout(t)
	var files []Material
	for _, item := range sampleMaterialPack().Files {
		files = append(files, Material{Path: item.Path, Body: item.Body})
	}
	cache, err := CacheRemoteMaterials(directory, spec, files)
	if err != nil {
		t.Fatal(err)
	}
	lessonPath := filepath.Join(cache.Directory, "lesson.md")
	if err := os.WriteFile(lessonPath, []byte("local note\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CacheRemoteMaterials(directory, spec, files); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("modified revision should be refused, got %v", err)
	}
	lesson, err := os.ReadFile(lessonPath)
	if err != nil || string(lesson) != "local note\n" {
		t.Fatalf("modified cache was overwritten: %q, %v", lesson, err)
	}
}

func TestCacheRemoteMaterialsRejectsSymlinkedRevisionDirectory(t *testing.T) {
	directory, spec := createMaterialCheckout(t)
	materialsRoot := filepath.Join(directory, ".learning", "materials")
	if err := os.Mkdir(materialsRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := t.TempDir()
	foreignFile := filepath.Join(foreign, "keep.txt")
	if err := os.WriteFile(foreignFile, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, filepath.Join(materialsRoot, spec.LessonRevision)); err != nil {
		t.Fatal(err)
	}
	var files []Material
	for _, item := range sampleMaterialPack().Files {
		files = append(files, Material{Path: item.Path, Body: item.Body})
	}
	if _, err := CacheRemoteMaterials(directory, spec, files); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("symlinked destination should be refused, got %v", err)
	}
	content, err := os.ReadFile(foreignFile)
	if err != nil || string(content) != "untouched" {
		t.Fatalf("external file was changed: %q, %v", content, err)
	}
}

func TestCacheRemoteMaterialsRequiresMatchingCheckoutAndPublicQuiz(t *testing.T) {
	directory, spec := createMaterialCheckout(t)
	manifestPath := filepath.Join(directory, ".learning", "manifest.json")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	changedManifest := strings.Replace(string(manifest), spec.TaskInstanceID, "11111111-1111-4111-8111-111111111111", 1)
	if err := os.WriteFile(manifestPath, []byte(changedManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	var files []Material
	for _, item := range sampleMaterialPack().Files {
		files = append(files, Material{Path: item.Path, Body: item.Body})
	}
	if _, err := CacheRemoteMaterials(directory, spec, files); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("modified checkout manifest should be refused, got %v", err)
	}

	directory, spec = createMaterialCheckout(t)
	files[len(files)-1].Body = `{"schema":"codechuman.quiz-view.v1","title":"Quiz","questions":[],"correct_choice_key":"a"}`
	if _, err := CacheRemoteMaterials(directory, spec, files); err == nil {
		t.Fatal("answer key was cached")
	}
	if _, err := os.Stat(filepath.Join(directory, ".learning", "materials", spec.LessonRevision)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed package left a visible cache: %v", err)
	}
}
