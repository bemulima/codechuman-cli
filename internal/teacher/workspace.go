package teacher

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"unicode/utf8"
)

// Read only the saved regular main.go, never the editor buffer, a symlink,
// directory tree, environment, Git metadata, or material/private test files.
func savedMain(workspace string) (string, error) {
	p := filepath.Join(workspace, "main.go")
	before, e := os.Lstat(p)
	if e != nil || !before.Mode().IsRegular() || before.Size() > 16<<10 {
		return "", errors.New("saved regular main.go up to 16 KiB required")
	}
	f, e := os.Open(p)
	if e != nil {
		return "", errors.New("saved main.go unreadable")
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(before, opened) {
		return "", errors.New("main.go changed during read")
	}
	b, e := io.ReadAll(io.LimitReader(f, (16<<10)+1))
	after, e2 := os.Lstat(p)
	if e != nil || e2 != nil || !os.SameFile(opened, after) || after.Size() != opened.Size() || !after.ModTime().Equal(opened.ModTime()) || len(b) > 16<<10 || !utf8.Valid(b) {
		return "", errors.New("unsafe or changed saved main.go")
	}
	return string(b), nil
}
func (c client) syncMain(ctx context.Context, workspace, sandboxID, key string) error {
	content, e := savedMain(workspace)
	if e != nil {
		return e
	}
	base := "/api/sandbox/v1/sandboxes/" + sandboxID
	var fence struct {
		Schema    string  `json:"schema"`
		SandboxID string  `json:"sandboxId"`
		Epoch     *uint64 `json:"fenceEpoch"`
	}
	if e = c.call(ctx, "GET", base+"/workspace-fence", nil, &fence); e != nil {
		return e
	}
	if fence.Schema != "sandbox-workspace-fence.v1" || fence.SandboxID != sandboxID || fence.Epoch == nil {
		return errors.New("invalid Sandbox fence")
	}
	var receipt struct {
		Events []struct {
			Event string `json:"event"`
		} `json:"events"`
	}
	if e = c.callHeaders(ctx, "PUT", base+"/files", map[string]string{"path": "main.go", "content": content}, &receipt, map[string]string{"Idempotency-Key": key, "X-Sandbox-Fence-Epoch": strconv.FormatUint(*fence.Epoch, 10)}); e != nil {
		return e
	}
	updated := false
	for _, event := range receipt.Events {
		if event.Event == "error" || event.Event == "write_forbidden" {
			return errors.New("Sandbox saved main.go denied")
		}
		updated = updated || event.Event == "file_updated"
	}
	if !updated {
		return errors.New("Sandbox save receipt missing")
	}
	var file struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if e = c.call(ctx, "GET", base+"/files?path=main.go", nil, &file); e != nil {
		return e
	}
	if file.Path != "main.go" || file.Content != content {
		return errors.New("Sandbox saved main.go verification failed")
	}
	return nil
}
