package teacher

import (
	"context"

	"errors"
	"net/http"
	"regexp"
)

type SubmitRequest struct {
	Token       string `json:"token"`
	Key         string `json:"idempotency_key"`
	Scope       string `json:"scope"`
	MilestoneID string `json:"milestone_id"`
	Replay      bool   `json:"replay"`
}
type OfficialReceipt struct {
	Schema          string `json:"schema"`
	ID              string `json:"id"`
	TaskID          string `json:"task_instance_id"`
	StudentID       string `json:"student_id"`
	Scope           string `json:"scope"`
	MilestoneID     string `json:"milestone_id,omitempty"`
	WorkspaceDigest string `json:"workspace_digest"`
	Status          string `json:"status"`
}
type SubmitResult struct {
	TaskID      string           `json:"task_instance_id"`
	Key         string           `json:"idempotency_key"`
	Scope       string           `json:"scope"`
	MilestoneID string           `json:"milestone_id,omitempty"`
	Uncertain   bool             `json:"uncertain"`
	Submission  *OfficialReceipt `json:"submission,omitempty"`
}

// Submit explicitly uses Practice's existing official command. Transport ambiguity
// returns the original key and scope; replay never uploads or invents a new intent.
func Submit(ctx context.Context, h *http.Client, origin, workspace string, in SubmitRequest) (SubmitResult, error) {
	out := SubmitResult{Key: in.Key, Scope: in.Scope, MilestoneID: in.MilestoneID}
	if !uuid.MatchString(in.Key) || (in.Scope != "MILESTONE" && in.Scope != "FINAL") {
		return out, errors.New("explicit scope and UUID idempotency key required")
	}
	if in.Scope == "FINAL" && in.MilestoneID != "" {
		return out, errors.New("final submission has no milestone")
	}
	c, task, e := taskClient(h, origin, workspace, in.Token)
	if e != nil {
		return out, e
	}
	out.TaskID = task
	var env struct {
		Instance struct {
			ID        string `json:"id"`
			StudentID string `json:"student_id"`
			Status    string `json:"status"`
			Current   *struct {
				ID string `json:"id"`
			} `json:"current_step"`
			Workspace struct {
				Schema    string `json:"schema"`
				ID        string `json:"sandbox_id"`
				Available bool   `json:"available"`
			} `json:"workspace_binding"`
		} `json:"instance"`
	}
	if e = c.call(ctx, "GET", "/api/practice-task/v2/task-instances/"+task, nil, &env); e != nil {
		return out, e
	}
	i := env.Instance
	if i.ID != task || !uuid.MatchString(i.StudentID) || i.Workspace.Schema != "practice-workspace-binding.v2" || !i.Workspace.Available || !uuid.MatchString(i.Workspace.ID) {
		return out, errors.New("owned provisioned task required")
	}
	if !in.Replay {
		if i.Status != "IN_PROGRESS" {
			return out, errors.New("task is not ready for a new submission")
		}
		if in.Scope == "MILESTONE" {
			if i.Current == nil || !uuid.MatchString(i.Current.ID) || (in.MilestoneID != "" && in.MilestoneID != i.Current.ID) {
				return out, errors.New("current frozen milestone required")
			}
			out.MilestoneID = i.Current.ID
		}
		if e = c.syncMain(ctx, workspace, i.Workspace.ID, in.Key); e != nil {
			return out, e
		}
	} else if in.Scope == "MILESTONE" && !uuid.MatchString(out.MilestoneID) {
		return out, errors.New("replay needs original milestone ID")
	}
	body := map[string]string{"schema": "practice-task-submit.v2", "scope": in.Scope}
	if out.MilestoneID != "" {
		body["milestone_id"] = out.MilestoneID
	}
	var receipt OfficialReceipt
	e = c.callHeaders(ctx, "POST", "/api/practice-task/v2/task-instances/"+task+"/submissions", body, &receipt, map[string]string{"Idempotency-Key": in.Key})
	if e != nil {
		out.Uncertain = true
		return out, nil
	}
	if receipt.Schema != "practice-task-submission.v2" || !uuid.MatchString(receipt.ID) || receipt.TaskID != task || receipt.StudentID != i.StudentID || receipt.Scope != out.Scope || receipt.MilestoneID != out.MilestoneID || !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(receipt.WorkspaceDigest) || !regexp.MustCompile(`^[A-Z][A-Z_]{0,31}$`).MatchString(receipt.Status) {
		out.Uncertain = true
		return out, nil
	}
	out.Submission = &receipt
	return out, nil
}
