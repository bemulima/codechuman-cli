package validation

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// PublicJavaScriptTests is explicitly opt-in: published public tests execute
// student code under the student's own OS account. This is never a sandbox
// and must never count as authoritative server-side validation or Mastery.
func PublicJavaScriptTests(ctx context.Context, checkoutDir string) Result {
	root, err := filepath.Abs(checkoutDir)
	if err != nil {
		return Result{Status: "FAIL", Details: "invalid checkout path"}
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Result{Status: "FAIL", Details: "checkout is unavailable"}
	}
	publicDir := root
	for _, component := range []string{".learning", "tests", "public"} {
		publicDir = filepath.Join(publicDir, component)
		info, err := os.Lstat(publicDir)
		if os.IsNotExist(err) {
			return Result{Status: "NO_TESTS", Details: "no locally cached public tests"}
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return Result{Status: "FAIL", Details: "unsafe public tests directory"}
		}
	}
	entries, err := os.ReadDir(publicDir)
	if err != nil {
		return Result{Status: "FAIL", Details: "cannot read public tests"}
	}
	var tests []string
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".test.js") {
			continue
		}
		info, err := os.Lstat(filepath.Join(publicDir, entry.Name()))
		if err != nil || !info.Mode().IsRegular() {
			return Result{Status: "FAIL", Details: "unsafe public test file"}
		}
		tests = append(tests, filepath.Join(publicDir, entry.Name()))
		if len(tests) > 64 {
			return Result{Status: "FAIL", Details: "too many public tests"}
		}
	}
	if len(tests) == 0 {
		return Result{Status: "NO_TESTS", Details: "no locally cached public tests"}
	}
	sort.Strings(tests)
	node, err := exec.LookPath("node")
	if err != nil {
		return Result{Status: "UNAVAILABLE", Details: "Node.js runtime is not installed"}
	}
	cmd := exec.CommandContext(ctx, node, append([]string{"--test"}, tests...)...)
	cmd.Dir = root
	output := &boundedOutput{limit: 8192}
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Result{Status: "FAIL", Details: "local public tests timed out or were cancelled"}
		}
		return Result{Status: "FAIL", Details: output.String()}
	}
	return Result{Status: "PASS", Details: fmt.Sprintf("%d public test file(s) passed", len(tests))}
}
