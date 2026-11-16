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

const task = "22222222-2222-4222-8222-222222222222"

func manifest(t *testing.T) string {
	t.Helper()
	p := t.TempDir()
	os.Mkdir(filepath.Join(p, ".learning"), 0700)
	os.WriteFile(filepath.Join(p, ".learning/manifest.json"), []byte(`{"schema":"codechuman.checkout.v1","task_instance_id":"`+task+`"}`), 0600)
	return p
}
func TestChatUsesOwnerActionAndDialogHistory(t *testing.T) {
	var posted bool
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing bearer")
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/practice-task/"):
			json.NewEncoder(w).Encode(map[string]any{"instance": map[string]string{"id": task, "learning_action_id": "11111111-1111-4111-8111-111111111111"}})
		case r.Method == "PUT":
			json.NewEncoder(w).Encode(map[string]string{"id": task, "context_type": "practice_task", "context_id": task})
		case r.Method == "POST":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["learning_action_id"] != "11111111-1111-4111-8111-111111111111" {
				t.Error("wrong owner action")
			}
			posted = true
			json.NewEncoder(w).Encode(map[string]string{"id": task})
		default:
			json.NewEncoder(w).Encode(map[string]any{"items": []Message{{ID: task, Author: "personal_teacher", Body: "Which method handles requests?", Reply: ptr(task)}}})
		}
	}))
	defer s.Close()
	v, e := Chat(context.Background(), s.Client(), s.URL, manifest(t), Request{Token: "secret", Body: "help", Key: NewKey()})
	if e != nil || !posted || len(v.Messages) != 1 {
		t.Fatal(v, e)
	}
}
func ptr(s string) *string { return &s }
func TestChatRejectsForeignBeforeDialogAndRedirect(t *testing.T) {
	for _, status := range []int{404, 307} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "https://foreign.invalid")
				w.WriteHeader(status)
			}))
			defer s.Close()
			_, e := Chat(context.Background(), s.Client(), s.URL, manifest(t), Request{Token: "secret"})
			if e == nil || calls != 1 {
				t.Fatal("authorization/redirect fail-open")
			}
		})
	}
}
