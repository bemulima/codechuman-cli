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

func TestSavedSyncRequiresPriorACKAndStopsBeforeChangedFence(t *testing.T) {
	for _, mode := range []string{"initial", "auto", "drift", "foreign", "missing_ack", "bad_ack"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			os.Mkdir(filepath.Join(dir, ".learning"), 0700)
			os.WriteFile(filepath.Join(dir, ".learning/manifest.json"), []byte(`{"schema":"codechuman.checkout.v1","task_instance_id":"`+task+`"}`), 0600)
			source := []byte("package main\nfunc main(){}\n")
			os.WriteFile(filepath.Join(dir, "main.go"), source, 0600)
			writes := 0
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Header.Get("Authorization") != "Bearer synthetic" {
					t.Error("missing auth")
				}
				switch {
				case strings.Contains(r.URL.Path, "task-instances"):
					if mode == "foreign" {
						w.WriteHeader(404)
						w.Write([]byte(`{}`))
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"instance": map[string]any{"id": task, "workspace_binding": map[string]any{"schema": "practice-workspace-binding.v2", "sandbox_id": task, "available": true}}})
				case strings.HasSuffix(r.URL.Path, "workspace-fence"):
					epoch := 7
					if mode == "drift" {
						epoch = 9
					}
					json.NewEncoder(w).Encode(map[string]any{"schema": "sandbox-workspace-fence.v1", "sandboxId": task, "fenceEpoch": epoch})
				case r.Method == "PUT":
					writes++
					if r.Header.Get("X-Sandbox-Fence-Epoch") != "7" {
						t.Error("missing expected fence")
					}
					var b map[string]string
					json.NewDecoder(r.Body).Decode(&b)
					if b["path"] != "main.go" || b["content"] != string(source) || len(b) != 2 {
						t.Error("wrong file projection")
					}
					ack := uint64(8)
					if mode == "bad_ack" {
						ack = 0
					}
					json.NewEncoder(w).Encode(map[string]any{"fenceEpoch": ack, "events": []any{map[string]string{"event": "file_updated"}}})
				default:
					json.NewEncoder(w).Encode(map[string]string{"path": "main.go", "content": string(source)})
				}
			}))
			defer s.Close()
			epoch := uint64(7)
			in := SyncRequest{Token: "synthetic", Key: task, ExpectedFence: &epoch}
			if mode == "initial" {
				in.ExpectedFence = nil
				in.Initialize = true
			}
			if mode == "missing_ack" {
				in.ExpectedFence = nil
			}
			out, e := SyncSaved(context.Background(), s.Client(), s.URL, dir, in)
			want := mode == "initial" || mode == "auto"
			if (e == nil) != want {
				t.Fatalf("out=%+v error=%v", out, e)
			}
			if want && (out.Fence != 8 || out.MainDigest != codeDigest(string(source))) {
				t.Fatal("incorrect ACK")
			}
			if !want && mode != "bad_ack" && writes != 0 {
				t.Fatal("unsafe write before admission")
			}
			raw, _ := os.ReadFile(filepath.Join(dir, "main.go"))
			if string(raw) != string(source) {
				t.Fatal("local code modified")
			}
		})
	}
}
