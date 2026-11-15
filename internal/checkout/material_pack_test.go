package checkout

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const materialTaskID = "22222222-2222-4222-8222-222222222222"

func materialSpec(origin string) Spec {
	return Spec{
		Schema:         schemaVersion,
		CourseID:       "88888888-8888-4888-8888-888888888888",
		CourseSlug:     "go-basics",
		LessonID:       "99999999-9999-4999-8999-999999999999",
		LessonNumber:   3,
		LessonRevision: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		TaskInstanceID: materialTaskID,
		SandboxID:      "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		CloneURL:       origin + gitExportRoutePrefix + materialTaskID + ".git",
	}
}

func publicMaterialFile(path, body string) remoteMaterialFile {
	sum := sha256.Sum256([]byte(body))
	return remoteMaterialFile{Path: path, Body: body, SHA256: fmt.Sprintf("sha256:%x", sum)}
}

func sampleMaterialPack() remoteMaterialsBundle {
	return remoteMaterialsBundle{
		Schema:         remoteMaterialsSchema,
		TaskInstanceID: materialTaskID,
		CourseID:       "88888888-8888-4888-8888-888888888888",
		LessonID:       "99999999-9999-4999-8999-999999999999",
		LessonRevision: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		Files: []remoteMaterialFile{
			publicMaterialFile("lesson.md", "# Lesson 3\n"),
			publicMaterialFile("task.md", "Implement answer().\n"),
			publicMaterialFile("examples/first.js", "const x = 1;\n"),
			publicMaterialFile("quiz.json", `{"schema":"codechuman.quiz-view.v1","title":"Quiz","questions":[{"key":"q1","prompt":"What?","choices":[{"key":"a","text":"A"},{"key":"b","text":"B"}]}]}`),
		},
	}
}

func writeMaterialReply(w http.ResponseWriter, pack any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	_ = json.NewEncoder(w).Encode(pack)
}

func TestResolveRemoteMaterialsUsesPinnedHTTPSAndChecksums(t *testing.T) {
	pack := sampleMaterialPack()
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != checkoutRoute+"/"+materialTaskID+"/materials" ||
			r.URL.RawQuery != "" || r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer opaque" ||
			r.Header.Get("Accept") != "application/json" || r.ContentLength > 0 {
			t.Errorf("invalid authenticated material request: method=%s path=%s", r.Method, r.URL.Path)
		}
		writeMaterialReply(w, pack)
	}))
	defer server.Close()
	files, err := ResolveRemoteMaterials(context.Background(), server.Client(), server.URL, "opaque", materialSpec(server.URL))
	if err != nil {
		t.Fatalf("authorized material pack rejected: %v", err)
	}
	if requests != 1 || len(files) != 4 || files[0].Path != "lesson.md" ||
		files[1].Body != "Implement answer().
" {
		t.Fatalf("unexpected files: %#v (requests=%d)", files, requests)
	}
}

func TestResolveRemoteMaterialsRejectsTamperingAndSecretQuiz(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*remoteMaterialsBundle)
	}{
		{"wrong TaskInstance", func(v *remoteMaterialsBundle) { v.TaskInstanceID = "11111111-1111-4111-8111-111111111111" }},
		{"wrong lesson revision", func(v *remoteMaterialsBundle) { v.LessonRevision = "cccccccc-cccc-4ccc-8ccc-cccccccccccc" }},
		{"checksum mismatch", func(v *remoteMaterialsBundle) { v.Files[0].Body += "tampered" }},
		{"private test", func(v *remoteMaterialsBundle) { v.Files = append(v.Files, publicMaterialFile("tests/private/hidden.js", "private")) }},
		{"traversal", func(v *remoteMaterialsBundle) { v.Files = append(v.Files, publicMaterialFile("../secret.txt", "private")) }},
		{"duplicate casefold path", func(v *remoteMaterialsBundle) { v.Files = append(v.Files, publicMaterialFile("examples/FIRST.js", "duplicate")) }},
		{"oversized file", func(v *remoteMaterialsBundle) { v.Files = append(v.Files, publicMaterialFile("examples/huge.js", strings.Repeat("x", 513*1024))) }},
		{"no lesson", func(v *remoteMaterialsBundle) { v.Files = v.Files[1:] }},
		{"quiz answer key", func(v *remoteMaterialsBundle) {
			v.Files[3] = publicMaterialFile("quiz.json", `{"schema":"codechuman.quiz-view.v1","title":"Quiz","questions":[{"key":"q1","prompt":"What?","choices":[{"key":"a","text":"A"},{"key":"b","text":"B"}],"correct_choice_key":"a"}]}`)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pack := sampleMaterialPack()
			tc.mutate(&pack)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeMaterialReply(w, pack)
			}))
			defer server.Close()
			if _, err := ResolveRemoteMaterials(context.Background(), server.Client(), server.URL, "opaque", materialSpec(server.URL)); err == nil {
				t.Fatal("untrusted material pack was accepted")
			}
		})
	}
}

func TestResolveRemoteMaterialsRejectsBadHTTPAndOrigin(t *testing.T) {
	var leaked bool
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("Authorization") != ""
	}))
	defer target.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/capture", http.StatusFound)
	}))
	defer redirect.Close()
	if _, err := ResolveRemoteMaterials(context.Background(), redirect.Client(), redirect.URL, "opaque", materialSpec(redirect.URL)); err == nil {
		t.Fatal("followed HTTP redirect")
	}
	if leaked {
		t.Fatal("bearer leaked across HTTP redirect")
	}
	if _, err := ResolveRemoteMaterials(context.Background(), redirect.Client(), "http://example.org", "opaque", materialSpec(redirect.URL)); err == nil {
		t.Fatal("accepted HTTP origin")
	}
	if _, err := ResolveRemoteMaterials(context.Background(), redirect.Client(), target.URL, "opaque", materialSpec(redirect.URL)); err == nil {
		t.Fatal("accepted a different Git checkout origin")
	}
	if _, err := ResolveRemoteMaterials(context.Background(), redirect.Client(), redirect.URL, "opaque token", materialSpec(redirect.URL)); err == nil {
		t.Fatal("accepted an unsafe bearer")
	}
}

func TestResolveRemoteMaterialsRequiresNoStore(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sampleMaterialPack())
	}))
	defer server.Close()
	if _, err := ResolveRemoteMaterials(context.Background(), server.Client(), server.URL, "opaque", materialSpec(server.URL)); err == nil {
		t.Fatal("accepted an uncacheable-protection-free materials response")
	}
}
