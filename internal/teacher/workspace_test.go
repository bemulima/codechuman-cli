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

func TestSavedMainRejectsSymlinkOversizeAndNonUTF8(t *testing.T) {
	p := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(outside, []byte("secret"), 0600)
	os.Symlink(outside, filepath.Join(p, "main.go"))
	if _, e := savedMain(p); e == nil {
		t.Fatal("symlink accepted")
	}
	os.Remove(filepath.Join(p, "main.go"))
	for _, b := range [][]byte{[]byte(strings.Repeat("x", 16385)), {255}} {
		os.WriteFile(filepath.Join(p, "main.go"), b, 0600)
		if _, e := savedMain(p); e == nil {
			t.Fatal("unsafe content accepted")
		}
	}
}
func TestSyncMainUsesFenceAndVerifiesSavedBytes(t *testing.T) {
	for _, deny := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved", true: "stale_fence"}[deny], func(t *testing.T) {
			p := t.TempDir()
			os.WriteFile(filepath.Join(p, "main.go"), []byte("package main\n"), 0600)
			calls := 0
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if r.Header.Get("Authorization") != "Bearer secret" {
					t.Error("missing Student authorization")
				}
				switch {
				case strings.HasSuffix(r.URL.Path, "workspace-fence"):
					json.NewEncoder(w).Encode(map[string]any{"schema": "sandbox-workspace-fence.v1", "sandboxId": task, "fenceEpoch": uint64(7)})
				case r.Method == "PUT":
					if r.Header.Get("X-Sandbox-Fence-Epoch") != "7" || r.Header.Get("Idempotency-Key") != "test-key" {
						t.Error("missing fenced idempotency")
					}
					var b map[string]string
					json.NewDecoder(r.Body).Decode(&b)
					if b["path"] != "main.go" || b["content"] != "package main\n" || len(b) != 2 {
						t.Error("wrong saved code payload")
					}
					if deny {
						w.WriteHeader(409)
					}
					w.Write([]byte(`{"events":[{"event":"file_updated"}]}`))
				default:
					json.NewEncoder(w).Encode(map[string]string{"path": "main.go", "content": "package main\n"})
				}
			}))
			defer s.Close()
			c := client{s.Client(), s.URL, "secret"}
			e := c.syncMain(context.Background(), p, task, "test-key")
			if (e != nil) != deny {
				t.Fatal(e)
			}
			want := 3
			if deny {
				want = 2
			}
			if calls != want {
				t.Fatal(calls)
			}
		})
	}
}
