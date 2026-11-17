package teacher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"time"
)

type CheckIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type CheckResult struct {
	TaskID         string       `json:"task_instance_id"`
	CheckID        string       `json:"request_id"`
	SnapshotDigest string       `json:"workspace_digest"`
	MainDigest     string       `json:"main_digest"`
	Outcome        string       `json:"outcome"`
	Issues         []CheckIssue `json:"issues"`
	Pending        bool         `json:"pending"`
	Stale          bool         `json:"stale"`
	Advisory       bool         `json:"advisory"`
}
type checkedRun struct {
	Schema         string       `json:"schema"`
	ID             string       `json:"request_id"`
	SandboxID      string       `json:"sandbox_id"`
	TaskID         string       `json:"task_instance_id"`
	StudentID      string       `json:"student_id"`
	RevisionID     string       `json:"task_revision_id"`
	RevisionDigest string       `json:"revision_digest"`
	Digest         string       `json:"workspace_digest"`
	MainDigest     string       `json:"main_digest"`
	Generation     string       `json:"generation_id"`
	Outcome        string       `json:"outcome"`
	Issues         []CheckIssue `json:"issues"`
	Stale          *bool        `json:"stale"`
}

func codeDigest(s string) string {
	v := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(v[:])
}

// Check runs the existing bounded Sandbox provider against saved code. It never
// creates an official Submission/checkpoint, freezes editing, or awards progress.
func Check(ctx context.Context, h *http.Client, origin, workspace string, input Request) (CheckResult, error) {
	fail := CheckResult{}
	c, task, e := taskClient(h, origin, workspace, input.Token)
	if e != nil {
		return fail, e
	}
	var envelope struct {
		Instance struct {
			ID             string `json:"id"`
			StudentID      string `json:"student_id"`
			RevisionID     string `json:"task_revision_id"`
			RevisionDigest string `json:"task_revision_digest"`
			Workspace      struct {
				Schema    string `json:"schema"`
				ID        string `json:"sandbox_id"`
				Available bool   `json:"available"`
			} `json:"workspace_binding"`
		} `json:"instance"`
	}
	if e = c.call(ctx, "GET", "/api/practice-task/v2/task-instances/"+task, nil, &envelope); e != nil {
		return fail, e
	}
	i := envelope.Instance
	if i.ID != task || !uuid.MatchString(i.StudentID) || !uuid.MatchString(i.RevisionID) || i.Workspace.Schema != "practice-workspace-binding.v2" || !i.Workspace.Available || !uuid.MatchString(i.Workspace.ID) {
		return fail, errors.New("owned real Sandbox task required")
	}
	key := input.Key
	if key == "" {
		key = NewKey()
	}
	if !uuid.MatchString(key) {
		return fail, errors.New("check idempotency key required")
	}
	saved, e := savedMain(workspace)
	if e != nil {
		return fail, e
	}
	expected := codeDigest(saved)
	if e = c.syncMain(ctx, workspace, i.Workspace.ID, key); e != nil {
		return fail, e
	}
	checkHTTP := *c.http
	checkHTTP.Timeout = 115 * time.Second
	c.http = &checkHTTP
	path := "/api/sandbox/v1/sandboxes/" + i.Workspace.ID + "/code-checks"
	var out checkedRun
	if e = c.callHeaders(ctx, "POST", path, map[string]string{"schema": "sandbox-code-check.v1"}, &out, map[string]string{"Idempotency-Key": key}); e != nil {
		return fail, e
	}
	for {
		digest := regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
		if out.Schema != "sandbox-code-check.v1" || out.ID != key || out.TaskID != task || out.StudentID != i.StudentID || out.SandboxID != i.Workspace.ID || out.RevisionID != i.RevisionID || out.RevisionDigest != i.RevisionDigest || !digest.MatchString(out.Digest) || out.MainDigest != expected || out.Stale == nil || !regexp.MustCompile(`^gen-[0-9a-f]{64}$`).MatchString(out.Generation) || len(out.Issues) > 7 {
			return fail, errors.New("code check identity or source mismatch")
		}
		result := CheckResult{TaskID: task, CheckID: key, SnapshotDigest: out.Digest, MainDigest: out.MainDigest, Outcome: out.Outcome, Issues: out.Issues, Pending: out.Outcome == "RUNNING", Stale: *out.Stale, Advisory: true}
		for _, issue := range out.Issues {
			if !regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`).MatchString(issue.Code) || len(issue.Message) > 512 {
				return fail, errors.New("invalid code check issue")
			}
		}
		if out.Outcome != "RUNNING" {
			if out.Outcome != "PASS" && out.Outcome != "FAIL" && out.Outcome != "ERROR" {
				return fail, errors.New("invalid code check outcome")
			}
			if out.Outcome == "PASS" && len(out.Issues) != 0 {
				return fail, errors.New("inconsistent code check outcome")
			}
			current, e := savedMain(workspace)
			result.Stale = result.Stale || e != nil || codeDigest(current) != expected
			return result, nil
		}
		select {
		case <-ctx.Done():
			return result, nil
		case <-time.After(500 * time.Millisecond):
		}
		out = checkedRun{}
		if e = c.call(ctx, "GET", path+"/latest", nil, &out); e != nil {
			return fail, e
		}
	}
}
