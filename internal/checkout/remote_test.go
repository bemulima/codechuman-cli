package checkout

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const remoteTaskID = "00000000-0000-4000-8000-000000000028"

func TestResolveRemoteCheckoutUsesScopedBearerAndSameOriginGitURL(t *testing.T) {
	token := "signed.jwt.token"
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/ide/checkouts" {
			t.Errorf("unexpected route %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+token ||
			r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing authenticated JSON request")
		}
		var body struct {
			Schema         string `json:"schema"`
			TaskInstanceID string `json:"task_instance_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
			body.Schema != "codechuman.checkout-request.v1" || body.TaskInstanceID != remoteTaskID {
			t.Errorf("unexpected checkout body: %+v (%v)", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		spec := specimen()
		spec.TaskInstanceID = remoteTaskID
		spec.CloneURL = server.URL + "/api/v1/ide/git/exports/ex-1.git"
		_ = json.NewEncoder(w).Encode(spec)
	}))
	defer server.Close()
	spec, err := ResolveRemote(context.Background(), server.Client(), server.URL, token, remoteTaskID)
	if err != nil || spec.CloneURL != server.URL+"/api/v1/ide/git/exports/ex-1.git" {
		t.Fatalf("authorized checkout resolution failed: %+v %v", spec, err)
	}
}

func TestResolveRemoteRejectsInsecureEndpointAndTokenLeakRedirect(t *testing.T) {
	if _, err := ResolveRemote(context.Background(), nil, "http://codechuman.example", "token", remoteTaskID); err == nil {
		t.Fatal("insecure API origin was accepted")
	}
	redirectTarget := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("bearer token leaked to redirect target")
	}))
	defer redirectTarget.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL+"/capture", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	_, err := ResolveRemote(context.Background(), redirect.Client(), redirect.URL, "secret", remoteTaskID)
	if err == nil || !strings.Contains(err.Error(), "HTTP 307") {
		t.Fatalf("expected fail-closed redirect refusal, got %v", err)
	}
}

func TestResolveRemoteRejectsOffOriginGitWithoutRevealingToken(t *testing.T) {
	const token = "top-secret-token"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spec := specimen()
		spec.TaskInstanceID = remoteTaskID
		spec.CloneURL = "https://foreign.example/api/v1/ide/git/exports/ex-1.git"
		_ = json.NewEncoder(w).Encode(spec)
	}))
	defer server.Close()
	_, err := ResolveRemote(context.Background(), server.Client(), server.URL, token, remoteTaskID)
	if err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("expected safe refusal of foreign clone host, got %v", err)
	}
}

func TestResolveRemoteRejectsForeignTaskIdentityAndInvalidID(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spec := specimen()
		spec.CloneURL = "https://" + r.Host + "/api/v1/ide/git/exports/ex-1.git"
		_ = json.NewEncoder(w).Encode(spec)
	}))
	defer server.Close()
	if _, err := ResolveRemote(context.Background(), server.Client(), server.URL, "token", "not-a-uuid"); err == nil {
		t.Fatal("invalid TaskInstance ID was accepted")
	}
	if _, err := ResolveRemote(context.Background(), server.Client(), server.URL, "token", remoteTaskID); err == nil {
		t.Fatal("mismatched TaskInstance binding was accepted")
	}
}
