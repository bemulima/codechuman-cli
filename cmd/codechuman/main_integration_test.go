package main

import (
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const integrationTaskID = "22222222-2222-4222-8222-222222222222"

// This runs the real checkout-remote command and native Git's dumb HTTPS
// transport against a disposable authenticated Gateway fixture. It does not
// impersonate real Student/Course/Practice owner proofs or their databases.
func TestCheckoutRemoteCommandClonesPinnedGitWithoutPersistingBearer(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("native Git is unavailable")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, root, "init", source)
	gitCommand(t, source, "config", "user.email", "ide28@example.invalid")
	gitCommand(t, source, "config", "user.name", "IDE28 Fixture")
	if err := os.WriteFile(filepath.Join(source, "app.js"), []byte("module.exports.answer = 42;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, source, "add", "app.js")
	gitCommand(t, source, "commit", "-m", "pin starter")
	bare := filepath.Join(root, "starter.git")
	gitCommand(t, root, "clone", "--bare", source, bare)
	gitCommand(t, bare, "update-server-info")

	const bearer = "fixture.student.token.not-persisted"
	const exportPrefix = "/api/v1/ide/git/exports/" + integrationTaskID + ".git/"
	var checkoutRequests, gitRequests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+bearer {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ide/checkouts":
			checkoutRequests.Add(1)
			var request struct {
				TaskInstanceID string `json:"task_instance_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.TaskInstanceID != integrationTaskID {
				t.Errorf("unexpected descriptor request: %+v, %v", request, err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "private, no-store")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schema": "codechuman.checkout.v1", "course_id": "88888888-8888-4888-8888-888888888888",
				"course_slug": "go-basics", "lesson_id": "99999999-9999-4999-8999-999999999999",
				"lesson_number": 3, "lesson_revision": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
				"task_instance_id": integrationTaskID, "sandbox_id": "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
				"git_url": "/api/v1/ide/git/exports/" + integrationTaskID + ".git",
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, exportPrefix):
			gitRequests.Add(1)
			http.StripPrefix(exportPrefix, http.FileServer(http.Dir(bare))).ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	caPath := filepath.Join(root, "ca.pem")
	if err := os.WriteFile(caPath, certificate, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_SSL_CAINFO", caPath)
	t.Setenv("SSL_CERT_FILE", caPath)
	t.Setenv("NO_PROXY", "127.0.0.1,localhost")
	checkoutRoot := filepath.Join(root, "checkout")
	args := []string{"checkout-remote", "--api-base", server.URL, "--task-instance", integrationTaskID, "--root", checkoutRoot, "--token-stdin"}
	for runNumber := 0; runNumber < 2; runNumber++ {
		withTokenStdin(t, bearer, func() {
			if err := run(args); err != nil {
				t.Fatalf("checkout-remote run %d: %v", runNumber, err)
			}
		})
		file := filepath.Join(checkoutRoot, "go-basics", "03", "app.js")
		if runNumber == 0 {
			content, err := os.ReadFile(file)
			if err != nil || string(content) != "module.exports.answer = 42;\n" {
				t.Fatalf("Git starter differs from pinned source: %q, %v", content, err)
			}
			if err := os.WriteFile(file, []byte("// unfinished student work\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		} else {
			content, _ := os.ReadFile(file)
			if string(content) != "// unfinished student work\n" {
				t.Fatal("reopening checkout overwrote student changes")
			}
		}
	}
	if checkoutRequests.Load() != 2 || gitRequests.Load() == 0 {
		t.Fatalf("unexpected descriptor/Git calls: %d/%d", checkoutRequests.Load(), gitRequests.Load())
	}
	for _, name := range []string{
		filepath.Join(checkoutRoot, "go-basics", "03", ".git", "config"),
		filepath.Join(checkoutRoot, "go-basics", "03", ".learning", "manifest.json"),
	} {
		content, err := os.ReadFile(name)
		if err != nil || strings.Contains(string(content), bearer) {
			t.Fatalf("credential leaked or manifest unavailable in %s: %v", name, err)
		}
	}
}

func withTokenStdin(t *testing.T, token string, action func()) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString(token + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	previous := os.Stdin
	os.Stdin = reader
	defer func() {
		os.Stdin = previous
		_ = reader.Close()
	}()
	action()
}

func gitCommand(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture git %v failed: %v; output: %s", args, err, output)
	}
}
