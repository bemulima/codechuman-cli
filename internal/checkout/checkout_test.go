package checkout

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type stubCloner struct {
	calls int
}

func (s *stubCloner) Clone(_ context.Context, _, target string) error {
	s.calls++
	if err := os.MkdirAll(filepath.Join(target, ".git", "info"), 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(target, "app.js"), []byte("function answer() {}\n"), 0o600)
}

func specimen() Spec {
	return Spec{
		Schema:         "codechuman.checkout.v1",
		CourseID:       "course-1",
		CourseSlug:     "js-beginner",
		LessonID:       "lesson-1",
		LessonNumber:   1,
		LessonRevision: "lesson-rev-1",
		TaskInstanceID: "task-1",
		SandboxID:      "sandbox-1",
		CloneURL:       "https://education.example.org/git/exports/export-1.git",
		Materials: []Material{{Path: "lesson.md", Body: "# Variables\n"},
			{Path: "examples/first.md", Body: "const x = 1\n"},
			{Path: "tests/public/check.js", Body: "// public test\n"}},
	}
}

func TestSpecRequiresHTTPSWithoutEmbeddedCredentials(t *testing.T) {
	for _, url := range []string{"http://education.example.org/repo.git", "ssh://education.example.org/repo.git", "https://student:secret@education.example.org/repo.git", "https://education.example.org/repo.git?token=secret"} {
		spec := specimen()
		spec.CloneURL = url
		if err := spec.Validate(); err == nil {
			t.Fatalf("unsafe URL %q was accepted", url)
		}
	}
	if err := specimen().Validate(); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
}

func TestSpecRejectsUntrustedPathsAndHiddenTests(t *testing.T) {
	for _, name := range []string{"../secret", "/absolute", ".git/config", "tests/private/secret.js", "tests/hidden.js", "examples/../../escape", "dir\\escape"} {
		spec := specimen()
		spec.Materials = []Material{{Path: name, Body: "data"}}
		if err := spec.Validate(); err == nil {
			t.Fatalf("unsafe material path %q was accepted", name)
		}
	}
}

func TestCheckoutDownloadsMaterialsAndRecordsImmutableIdentity(t *testing.T) {
	root := t.TempDir()
	cloner := &stubCloner{}
	spec := specimen()
	result, err := Checkout(context.Background(), root, spec, cloner)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reused || cloner.calls != 1 {
		t.Fatalf("expected one new clone: %+v, calls=%d", result, cloner.calls)
	}
	for _, name := range []string{".git", "app.js", ".learning/lesson.md", ".learning/examples/first.md", ".learning/tests/public/check.js", ".learning/manifest.json"} {
		if _, err := os.Stat(filepath.Join(result.Directory, name)); err != nil {
			t.Fatalf("missing checkout file %s: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(result.Directory, ".learning", "manifest.json"))
	if err != nil || !strings.Contains(string(data), `"task_instance_id": "task-1"`) {
		t.Fatalf("bad manifest: %s, %v", data, err)
	}
}

func TestCheckoutReusesWithoutOverwritingStudentChanges(t *testing.T) {
	root := t.TempDir()
	cloner := &stubCloner{}
	spec := specimen()
	first, err := Checkout(context.Background(), root, spec, cloner)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(first.Directory, "app.js")
	if err := os.WriteFile(file, []byte("// my unfinished solution\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := Checkout(context.Background(), root, spec, cloner)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(file)
	if !second.Reused || cloner.calls != 1 || string(content) != "// my unfinished solution\n" {
		t.Fatalf("checkout overwrote local work: %+v, calls=%d, content=%q", second, cloner.calls, content)
	}
}

func TestCheckoutDoesNotOverwriteDifferentRevision(t *testing.T) {
	root := t.TempDir()
	cloner := &stubCloner{}
	spec := specimen()
	if _, err := Checkout(context.Background(), root, spec, cloner); err != nil {
		t.Fatal(err)
	}
	spec.LessonRevision = "lesson-rev-2"
	_, err := Checkout(context.Background(), root, spec, cloner)
	if !errors.Is(err, ErrRevisionConflict) || cloner.calls != 1 {
		t.Fatalf("expected immutable revision fence, got %v (clones %d)", err, cloner.calls)
	}
}

func TestCheckoutNeverAdoptsUnknownExistingDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "js-beginner", "01")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	cloner := &stubCloner{}
	_, err := Checkout(context.Background(), root, specimen(), cloner)
	if !errors.Is(err, ErrTargetOccupied) || cloner.calls != 0 {
		t.Fatalf("expected non-destructive refusal, got %v", err)
	}
}

func TestCheckoutRejectsSymlinkedExistingDirectory(t *testing.T) {
	root := t.TempDir()
	foreign := t.TempDir()
	courseDir := filepath.Join(root, "js-beginner")
	if err := os.MkdirAll(courseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, filepath.Join(courseDir, "01")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	cloner := &stubCloner{}
	_, err := Checkout(context.Background(), root, specimen(), cloner)
	if !errors.Is(err, ErrTargetOccupied) || cloner.calls != 0 {
		t.Fatalf("expected symlink refusal, got %v (clones %d)", err, cloner.calls)
	}
}

func TestWorkspaceRootRejectsPathsCanonicalizingToFilesystemRoot(t *testing.T) {
	root := string(filepath.Separator)
	for _, path := range []string{root, filepath.Join(root, "tmp", "..") + string(filepath.Separator)} {
		_, err := WorkspaceRoot(path)
		if err == nil {
			t.Fatalf("filesystem root alias %q was accepted", path)
		}
	}
}
