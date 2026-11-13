package checkout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const schemaVersion = "codechuman.checkout.v1"

var (
	ErrRevisionConflict  = errors.New("checkout revision does not match the local workspace")
	ErrTargetOccupied    = errors.New("checkout directory already exists without a matching manifest")
	ErrCheckoutInProgress = errors.New("checkout for this lesson is already in progress")
	ErrReservedPath      = errors.New("repository contains a reserved .learning path")
	safeSlug             = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,79}$`)
)

// Material is a public Course-owned lesson asset. Private tests and answers
// never belong in a student checkout specification.
type Material struct {
	Path string `json:"path"`
	Body string `json:"body"`
}

// Spec is a version-pinned, server-authorized checkout instruction. The client
// validates untrusted transport fields before touching the filesystem.
type Spec struct {
	Schema         string     `json:"schema"`
	CourseID       string     `json:"course_id"`
	CourseSlug     string     `json:"course_slug"`
	LessonID       string     `json:"lesson_id"`
	LessonNumber   int        `json:"lesson_number"`
	LessonRevision string     `json:"lesson_revision"`
	TaskInstanceID string     `json:"task_instance_id"`
	SandboxID      string     `json:"sandbox_id"`
	CloneURL       string     `json:"clone_url"`
	Materials      []Material `json:"materials"`
}

// Manifest excludes potentially sensitive or large content bodies. It can be
// safely read by the VS Code and Neovim clients without fetching Course again.
type Manifest struct {
	Schema         string `json:"schema"`
	CourseID       string `json:"course_id"`
	CourseSlug     string `json:"course_slug"`
	LessonID       string `json:"lesson_id"`
	LessonNumber   int    `json:"lesson_number"`
	LessonRevision string `json:"lesson_revision"`
	TaskInstanceID string `json:"task_instance_id"`
	SandboxID      string `json:"sandbox_id"`
}

func (s Spec) Manifest() Manifest {
	return Manifest{
		Schema: s.Schema, CourseID: s.CourseID, CourseSlug: s.CourseSlug,
		LessonID: s.LessonID, LessonNumber: s.LessonNumber,
		LessonRevision: s.LessonRevision, TaskInstanceID: s.TaskInstanceID,
		SandboxID: s.SandboxID,
	}
}

func (s Spec) Validate() error {
	if s.Schema != schemaVersion {
		return fmt.Errorf("unsupported checkout schema %q", s.Schema)
	}
	if !safeSlug.MatchString(s.CourseSlug) || s.CourseSlug == "." || s.CourseSlug == ".." {
		return fmt.Errorf("invalid course slug %q", s.CourseSlug)
	}
	if s.LessonNumber < 1 || s.LessonNumber > 9999 {
		return errors.New("lesson_number must be between 1 and 9999")
	}
	for name, value := range map[string]string{
		"course_id": s.CourseID, "lesson_id": s.LessonID,
		"lesson_revision": s.LessonRevision, "task_instance_id": s.TaskInstanceID,
		"sandbox_id": s.SandboxID,
	} {
		if strings.TrimSpace(value) == "" || len(value) > 256 {
			return fmt.Errorf("invalid %s", name)
		}
	}
	u, err := url.Parse(s.CloneURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.Path == "" {
		return errors.New("clone_url must be HTTPS without embedded credentials or query tokens")
	}
	seen := map[string]bool{}
	totalBytes := 0
	for _, item := range s.Materials {
		if !allowedMaterialPath(item.Path) {
			return fmt.Errorf("unsafe or non-public material path %q", item.Path)
		}
		key := strings.ToLower(item.Path)
		if seen[key] {
			return fmt.Errorf("duplicate material path %q", item.Path)
		}
		seen[key] = true
		if len(item.Body) > 512*1024 {
			return fmt.Errorf("material %q is oversized", item.Path)
		}
		totalBytes += len(item.Body)
		if totalBytes > 4*1024*1024 {
			return errors.New("public material pack is oversized")
		}
	}
	return nil
}

func allowedMaterialPath(name string) bool {
	if name == "" || strings.ContainsAny(name, "\\\x00\r\n") || path.IsAbs(name) || path.Clean(name) != name {
		return false
	}
	for _, component := range strings.Split(name, "/") {
		if component == "" || component == "." || component == ".." || strings.HasPrefix(component, ".") || strings.Contains(component, ":") {
			return false
		}
	}
	return name == "lesson.md" || name == "task.md" || name == "quiz.json" ||
		strings.HasPrefix(name, "examples/") || strings.HasPrefix(name, "tests/public/")
}

func ReadSpec(reader io.Reader) (Spec, error) {
	var s Spec
	d := json.NewDecoder(io.LimitReader(reader, 8*1024*1024))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return Spec{}, err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return Spec{}, errors.New("checkout spec must contain exactly one JSON object")
	}
	if err := s.Validate(); err != nil {
		return Spec{}, err
	}
	return s, nil
}

func WorkspaceRoot(override string) (string, error) {
	root := strings.TrimSpace(override)
	if root == "" {
		root = strings.TrimSpace(os.Getenv("CODECHUMAN_WORKSPACE_ROOT"))
	}
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".codechuman")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if filepath.Dir(absolute) == absolute {
		return "", errors.New("filesystem root cannot be a Codechuman workspace")
	}
	return absolute, nil
}

type Cloner interface {
	Clone(ctx context.Context, repoURL string, target string) error
}

type GitCloner struct {
	Binary      string
	BearerToken string // Ephemeral remote-checkout credential, never persisted.
}

func (g GitCloner) Clone(ctx context.Context, repoURL, target string) error {
	bin := g.Binary
	if bin == "" {
		bin = "git"
	}
	cmd := exec.CommandContext(ctx, bin, "clone", "--depth", "1", "--no-tags", "--", repoURL, target)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if g.BearerToken != "" {
		remote, err := url.Parse(repoURL)
		if err != nil || remote.Scheme != "https" || remote.Hostname() == "" ||
			remote.User != nil || remote.RawQuery != "" || remote.Fragment != "" ||
			!strings.HasPrefix(remote.Path, gitExportRoutePrefix) || !strings.HasSuffix(remote.Path, ".git") {
			return errors.New("invalid authenticated Git HTTPS export URL")
		}
		if len(g.BearerToken) > 8192 || strings.ContainsAny(g.BearerToken, "\r\n\x00\t ") {
			return errors.New("invalid Git HTTPS bearer token")
		}
		// Pass the scoped HTTP header through Git's environment configuration,
		// never through argv, remote URL, persistent .git/config or logs.
		// Git traces, credential helpers, prompts and redirects are disabled.
		filtered := make([]string, 0, len(cmd.Env)+9)
		for _, item := range cmd.Env {
			key, _, _ := strings.Cut(item, "=")
			if strings.HasPrefix(key, "GIT_CONFIG_") || strings.HasPrefix(key, "GIT_TRACE") ||
				key == "GIT_CURL_VERBOSE" || key == "GIT_ASKPASS" || key == "SSH_ASKPASS" {
				continue
			}
			filtered = append(filtered, item)
		}
		cmd.Env = append(filtered,
			"GIT_CONFIG_COUNT=3",
			"GIT_CONFIG_KEY_0=http.followRedirects",
			"GIT_CONFIG_VALUE_0=false",
			"GIT_CONFIG_KEY_1=http."+repoURL+".extraHeader",
			"GIT_CONFIG_VALUE_1=Authorization: Bearer "+g.BearerToken,
			"GIT_CONFIG_KEY_2=credential.helper",
			"GIT_CONFIG_VALUE_2=",
			"GIT_ASKPASS=", "SSH_ASKPASS=")
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Git can include sensitive helper output; do not persist or surface it.
		_ = output
		return fmt.Errorf("git clone failed: %w", err)
	}
	return nil
}

type Result struct {
	Directory string `json:"directory"`
	Reused    bool   `json:"reused"`
}

func Checkout(ctx context.Context, root string, spec Spec, cloner Cloner) (Result, error) {
	if err := spec.Validate(); err != nil {
		return Result{}, err
	}
	if cloner == nil {
		return Result{}, errors.New("git cloner is required")
	}
	root, err := WorkspaceRoot(root)
	if err != nil {
		return Result{}, err
	}
	courseDir := filepath.Join(root, spec.CourseSlug)
	if info, err := os.Lstat(courseDir); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return Result{}, ErrTargetOccupied
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}
	if err := os.MkdirAll(courseDir, 0o700); err != nil {
		return Result{}, err
	}
	if info, err := os.Lstat(courseDir); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Result{}, ErrTargetOccupied
	}
	lessonName := fmt.Sprintf("%02d", spec.LessonNumber)
	lockPath := filepath.Join(courseDir, "."+lessonName+".checkout.lock")
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return Result{}, ErrCheckoutInProgress
	}
	if err != nil {
		return Result{}, err
	}
	_ = lock.Close()
	defer os.Remove(lockPath)

	target := filepath.Join(courseDir, lessonName)
	if info, err := os.Lstat(target); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return Result{}, ErrTargetOccupied
		}
		data, err := os.ReadFile(filepath.Join(target, ".learning", "manifest.json"))
		if err != nil {
			return Result{}, ErrTargetOccupied
		}
		var existing Manifest
		if err := json.Unmarshal(data, &existing); err != nil {
			return Result{}, ErrTargetOccupied
		}
		if existing != spec.Manifest() {
			return Result{}, ErrRevisionConflict
		}
		return Result{Directory: target, Reused: true}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}

	tmp, err := os.MkdirTemp(courseDir, ".checkout-tmp-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tmp) // Only our unique, newly created temporary directory.
	working := filepath.Join(tmp, "work")
	if err := cloner.Clone(ctx, spec.CloneURL, working); err != nil {
		return Result{}, err
	}
	if _, err := os.Lstat(filepath.Join(working, ".learning")); err == nil {
		return Result{}, ErrReservedPath
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}
	privateDir := filepath.Join(working, ".learning")
	if err := os.Mkdir(privateDir, 0o700); err != nil {
		return Result{}, err
	}
	for _, asset := range spec.Materials {
		file := filepath.Join(privateDir, filepath.FromSlash(asset.Path))
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return Result{}, err
		}
		if err := os.WriteFile(file, []byte(asset.Body), 0o600); err != nil {
			return Result{}, err
		}
	}
	manifestBytes, err := json.MarshalIndent(spec.Manifest(), "", "  ")
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(filepath.Join(privateDir, "manifest.json"), append(manifestBytes, '\n'), 0o600); err != nil {
		return Result{}, err
	}
	exclude := filepath.Join(working, ".git", "info", "exclude")
	if f, err := os.OpenFile(exclude, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600); err == nil {
		if _, err := f.WriteString("\n/.learning/\n"); err != nil {
			_ = f.Close()
			return Result{}, err
		}
		if err := f.Close(); err != nil {
			return Result{}, err
		}
	} else {
		return Result{}, err
	}
	if _, err := os.Lstat(target); err == nil {
		return Result{}, ErrTargetOccupied
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}
	if err := os.Rename(working, target); err != nil {
		return Result{}, err
	}
	return Result{Directory: target}, nil
}
