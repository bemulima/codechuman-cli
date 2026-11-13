package validation

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Result is advisory local feedback, never an authoritative Mastery decision.
type Result struct {
	Status  string `json:"status"`
	Details string `json:"details,omitempty"`
}

func JavaScriptSyntax(ctx context.Context, checkoutDir, relativeFile string) Result {
	if filepath.IsAbs(relativeFile) || relativeFile == "" ||
		strings.ContainsAny(relativeFile, "\\\x00\r\n") ||
		strings.HasPrefix(relativeFile, ".") ||
		strings.Contains("/"+filepath.ToSlash(relativeFile)+"/", "/../") ||
		strings.Contains("/"+filepath.ToSlash(relativeFile)+"/", "/.learning/") ||
		strings.Contains("/"+filepath.ToSlash(relativeFile)+"/", "/.git/") ||
		filepath.Ext(relativeFile) != ".js" {
		return Result{Status: "FAIL", Details: "unsafe or unsupported validation target"}
	}
	root, err := filepath.Abs(checkoutDir)
	if err != nil {
		return Result{Status: "FAIL", Details: "invalid workspace path"}
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Result{Status: "FAIL", Details: "workspace is unavailable"}
	}
	file := filepath.Join(root, relativeFile)
	file, err = filepath.EvalSymlinks(file)
	if err != nil {
		return Result{Status: "FAIL", Details: "validation file is missing"}
	}
	rel, err := filepath.Rel(root, file)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return Result{Status: "FAIL", Details: "file is outside the checkout"}
	}
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() {
		return Result{Status: "FAIL", Details: "validation target is not a regular file"}
	}
	bin, err := exec.LookPath("node")
	if err != nil {
		return Result{Status: "UNAVAILABLE", Details: "Node.js runtime is not installed"}
	}
	cmd := exec.CommandContext(ctx, bin, "--check", file)
	cmd.Dir = root
	output := &boundedOutput{limit: 8192}
	cmd.Stdout, cmd.Stderr = output, output
	err = cmd.Run()
	if err == nil {
		return Result{Status: "PASS"}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return Result{Status: "FAIL", Details: "local syntax check timed out"}
	}
	return Result{Status: "FAIL", Details: output.String()}
}
