package teacher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfficialSubmitUsesFrozenOwnerContractAndSameKeyReplay(t *testing.T) {
	for _, mode := range []string{"normal", "foreign", "conflict", "uncertain", "replay"} {
		t.Run(mode, func(t *testing.T) {
			root := manifest(t)
			source := "package main\nfunc main(){}"
			os.WriteFile(filepath.Join(root, "main.go"), []byte(source), 0600)
			task := "22222222-2222-4222-8222-222222222222"
			student := "11111111-1111-4111-8111-111111111111"
			sb := "77777777-7777-4777-8777-777777777777"
			step := "33333333-3333-4333-8333-333333333333"
			key := NewKey()
			writes, posts := 0, 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Header.Get("Authorization") != "Bearer synthetic-token" {
					t.Error("missing bearer")
				}
				switch {
				case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/practice-task/"):
					if mode == "foreign" {
						w.WriteHeader(404)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"instance": map[string]any{"id": task, "student_id": student, "status": "IN_PROGRESS", "current_step": map[string]string{"id": step}, "workspace_binding": map[string]any{"schema": "practice-workspace-binding.v2", "sandbox_id": sb, "available": true}}})
				case strings.HasSuffix(r.URL.Path, "workspace-fence"):
					w.Write([]byte(`{"schema":"sandbox-workspace-fence.v1","sandboxId":"77777777-7777-4777-8777-777777777777","fenceEpoch":1}`))
				case r.Method == "PUT":
					writes++
					if mode == "conflict" {
						w.WriteHeader(409)
						return
					}
					w.Write([]byte(`{"events":[{"event":"file_updated"}]}`))
				case r.Method == "GET":
					json.NewEncoder(w).Encode(map[string]string{"path": "main.go", "content": source})
				case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/submissions"):
					posts++
					if r.Header.Get("Idempotency-Key") != key {
						t.Error("new key invented")
					}
					var b map[string]string
					json.NewDecoder(r.Body).Decode(&b)
					if len(b) != 3 || b["schema"] != "practice-task-submit.v2" || b["scope"] != "MILESTONE" || b["milestone_id"] != step {
						t.Error("official contract changed")
					}
					if mode == "uncertain" {
						w.WriteHeader(503)
						return
					}
					w.WriteHeader(202)
					json.NewEncoder(w).Encode(OfficialReceipt{Schema: "practice-task-submission.v2", ID: "44444444-4444-4444-8444-444444444444", TaskID: task, StudentID: student, Scope: "MILESTONE", MilestoneID: step, WorkspaceDigest: "sha256:" + strings.Repeat("a", 64), Status: "VALIDATING"})
				default:
					t.Error("unexpected endpoint")
				}
			}))
			defer server.Close()
			in := SubmitRequest{Token: "synthetic-token", Key: key, Scope: "MILESTONE"}
			if mode == "replay" {
				in.Replay = true
				in.MilestoneID = step
			}
			out, e := Submit(context.Background(), server.Client(), server.URL, root, in)
			if mode == "foreign" || mode == "conflict" {
				if e == nil || posts != 0 {
					t.Fatal("Submit after failed ownership/save")
				}
				return
			}
			if e != nil || out.Key != key || posts != 1 {
				t.Fatal(out, e)
			}
			if mode == "uncertain" {
				if !out.Uncertain || out.Submission != nil {
					t.Fatal("fabricated receipt")
				}
			} else if out.Submission == nil || out.Uncertain {
				t.Fatal("receipt missing")
			}
			if mode == "replay" && writes != 0 {
				t.Fatal("replay rewrote frozen code")
			}
			if b, _ := os.ReadFile(filepath.Join(root, "main.go")); string(b) != source {
				t.Fatal("local code changed")
			}
		})
	}
}
