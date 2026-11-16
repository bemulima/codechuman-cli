package teacher

import (
	"context"
	"errors"
	"net/http"
	"strconv"
)

type SyncRequest struct {
	Token         string  `json:"token"`
	Key           string  `json:"idempotency_key"`
	Initialize    bool    `json:"initialize"`
	ExpectedFence *uint64 `json:"expected_fence_epoch,omitempty"`
}
type SyncResult struct {
	Schema     string `json:"schema"`
	TaskID     string `json:"task_instance_id"`
	SandboxID  string `json:"sandbox_id"`
	Fence      uint64 `json:"fence_epoch"`
	MainDigest string `json:"main_digest"`
}

// SyncSaved is an explicit upload, or an automatic compare-and-write against a
// previous acknowledged fence. No check, Teacher, Git or official Submit runs.
func SyncSaved(ctx context.Context, h *http.Client, origin, workspace string, in SyncRequest) (SyncResult, error) {
	fail := SyncResult{}
	if !uuid.MatchString(in.Key) || (in.Initialize == (in.ExpectedFence != nil)) {
		return fail, errors.New("explicit initialization or acknowledged fence required")
	}
	c, task, e := taskClient(h, origin, workspace, in.Token)
	if e != nil {
		return fail, e
	}
	var owned struct {
		Instance struct {
			ID        string `json:"id"`
			Workspace struct {
				Schema    string `json:"schema"`
				ID        string `json:"sandbox_id"`
				Available bool   `json:"available"`
			} `json:"workspace_binding"`
		} `json:"instance"`
	}
	if e = c.call(ctx, "GET", "/api/practice-task/v2/task-instances/"+task, nil, &owned); e != nil {
		return fail, e
	}
	b := owned.Instance.Workspace
	if owned.Instance.ID != task || b.Schema != "practice-workspace-binding.v2" || !b.Available || !uuid.MatchString(b.ID) {
		return fail, errors.New("owned Sandbox binding required")
	}
	content, e := savedMain(workspace)
	if e != nil {
		return fail, e
	}
	base := "/api/sandbox/v1/sandboxes/" + b.ID
	var fence struct {
		Schema    string  `json:"schema"`
		SandboxID string  `json:"sandboxId"`
		Epoch     *uint64 `json:"fenceEpoch"`
	}
	if e = c.call(ctx, "GET", base+"/workspace-fence", nil, &fence); e != nil {
		return fail, e
	}
	if fence.Schema != "sandbox-workspace-fence.v1" || fence.SandboxID != b.ID || fence.Epoch == nil || *fence.Epoch == 0 {
		return fail, errors.New("confirmed atomic fence required")
	}
	if in.ExpectedFence != nil && *in.ExpectedFence != *fence.Epoch {
		return fail, errors.New("Sandbox fence changed; automatic sync halted")
	}
	var receipt struct {
		Fence  *uint64 `json:"fenceEpoch"`
		Events []struct {
			Event string `json:"event"`
		} `json:"events"`
	}
	if e = c.callHeaders(ctx, "PUT", base+"/files", map[string]string{"path": "main.go", "content": content}, &receipt, map[string]string{"Idempotency-Key": in.Key, "X-Sandbox-Fence-Epoch": strconv.FormatUint(*fence.Epoch, 10)}); e != nil {
		return fail, e
	}
	updated := false
	for _, event := range receipt.Events {
		if event.Event == "error" || event.Event == "write_forbidden" {
			return fail, errors.New("Sandbox sync denied")
		}
		updated = updated || event.Event == "file_updated"
	}
	if !updated || receipt.Fence == nil || *receipt.Fence < *fence.Epoch {
		return fail, errors.New("atomic sync ACK missing")
	}
	var file struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if e = c.call(ctx, "GET", base+"/files?path=main.go", nil, &file); e != nil {
		return fail, e
	}
	if file.Path != "main.go" || file.Content != content {
		return fail, errors.New("saved source ACK differs")
	}
	return SyncResult{Schema: "codechuman.saved-sync.v1", TaskID: task, SandboxID: b.ID, Fence: *receipt.Fence, MainDigest: codeDigest(content)}, nil
}
