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

func TestCheckUsesSavedMainAndAdvisorySandboxOnly(t *testing.T) {
	for _, mode := range []string{"PASS", "FAIL", "ERROR", "foreign", "drift", "local_edit"} {
		t.Run(mode, func(t *testing.T) {
			p := manifest(t)
			source := "package main\nfunc main(){}"
			os.WriteFile(filepath.Join(p, "main.go"), []byte(source), 0600)
			sandbox := "77777777-7777-4777-8777-777777777777"
			student := "11111111-1111-4111-8111-111111111111"
			revision := "33333333-3333-4333-8333-333333333333"
			revisionDigest := "sha256:" + strings.Repeat("b", 64)
			key := NewKey()
			runCalls := 0
			mutations := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Header.Get("Authorization") != "Bearer synthetic-token" {
					t.Error("missing bearer")
				}
				switch {
				case strings.HasPrefix(r.URL.Path, "/api/practice-task/"):
					if r.Method != "GET" {
						t.Fatal("run created an official mutation")
					}
					if mode == "foreign" {
						w.WriteHeader(404)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"instance": map[string]any{"id": task, "student_id": student, "task_revision_id": revision, "task_revision_digest": revisionDigest, "workspace_binding": map[string]any{"schema": "practice-workspace-binding.v2", "sandbox_id": sandbox, "available": true}}})
				case strings.HasSuffix(r.URL.Path, "workspace-fence"):
					json.NewEncoder(w).Encode(map[string]any{"schema": "sandbox-workspace-fence.v1", "sandboxId": sandbox, "fenceEpoch": 1})
				case strings.HasSuffix(r.URL.Path, "/files") && r.Method == "PUT":
					mutations++
					if r.Header.Get("Idempotency-Key") != key || r.Header.Get("X-Sandbox-Fence-Epoch") != "1" {
						t.Error("unfenced save")
					}
					w.Write([]byte(`{"events":[{"event":"file_updated"}]}`))
				case strings.HasSuffix(r.URL.Path, "/files"):
					json.NewEncoder(w).Encode(map[string]string{"path": "main.go", "content": source})
				case strings.HasSuffix(r.URL.Path, "/code-checks"):
					runCalls++
					if r.Method != "POST" || r.Header.Get("Idempotency-Key") != key {
						t.Error("wrong run boundary")
					}
					status := mode
					if mode == "drift" || mode == "local_edit" {
						status = "PASS"
					}
					issues := []CheckIssue{}
					if status == "FAIL" {
						issues = append(issues, CheckIssue{Code: "HTTP_HEALTH_BODY", Message: "Public criterion failed."})
					}
					if status == "ERROR" {
						issues = append(issues, CheckIssue{Code: "SOURCE_PROFILE_REJECTED", Message: "Technical error."})
					}
					row := map[string]any{"schema": "sandbox-code-check.v1", "request_id": key, "sandbox_id": sandbox, "task_instance_id": task, "student_id": student, "task_revision_id": revision, "revision_digest": revisionDigest, "workspace_digest": "sha256:" + strings.Repeat("c", 64), "main_digest": codeDigest(source), "generation_id": "gen-" + strings.Repeat("d", 64), "outcome": status, "issues": issues, "stale": false}
					if mode == "drift" {
						row["student_id"] = "foreign"
					}
					if mode == "local_edit" {
						os.WriteFile(filepath.Join(p, "main.go"), []byte(source+"\n// changed during run"), 0600)
					}
					json.NewEncoder(w).Encode(row)
				default:
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
			}))
			defer server.Close()
			out, e := Check(context.Background(), server.Client(), server.URL, p, Request{Token: "synthetic-token", Key: key})
			if mode == "foreign" || mode == "drift" {
				if e == nil {
					t.Fatal("bad ownership/identity accepted")
				}
				if mode == "foreign" && (runCalls != 0 || mutations != 0) {
					t.Fatal("foreign source uploaded")
				}
				return
			}
			if e != nil || !out.Advisory || out.Pending || runCalls != 1 {
				t.Fatal(out, e)
			}
			if mode == "local_edit" {
				if !out.Stale {
					t.Fatal("changed local code claimed checked")
				}
			} else if out.Outcome != mode || out.Stale {
				t.Fatal(out)
			}
		})
	}
}
