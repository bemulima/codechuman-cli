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

func validRemoteDescriptor() remoteDescriptor {
	return remoteDescriptor{
		Schema:         "codechuman.checkout.v1",
		CourseID:       "88888888-8888-4888-8888-888888888888",
		CourseSlug:     "go-basics",
		LessonID:       "99999999-9999-4999-8999-999999999999",
		LessonNumber:   3,
		LessonRevision: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		TaskInstanceID: remoteTaskID,
		SandboxID:      "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		GitURL:         gitExportRoutePrefix + remoteTaskID + ".git",
	}
}

func writeRemoteDescriptor(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	_ = json.NewEncoder(w).Encode(value)
}

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
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) != 1 ||
			string(body["task_instance_id"]) != `"`+remoteTaskID+`"` {
			t.Errorf("checkout request must contain only task_instance_id: %+v (%v)", body, err)
		}
		writeRemoteDescriptor(w, validRemoteDescriptor())
	}))
	defer server.Close()
	spec, err := ResolveRemote(context.Background(), server.Client(), server.URL, token, remoteTaskID)
	if err != nil || spec.CloneURL != server.URL+gitExportRoutePrefix+remoteTaskID+".git" ||
		len(spec.Materials) != 0 || spec.CourseSlug != "go-basics" || spec.LessonNumber != 3 {
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

func TestResolveRemoteRejectsUnpinnedOrNonRelativeGitWithoutRevealingToken(t *testing.T) {
	const token = "top-secret-token"
	for _, gitURL := range []string{
		"https://foreign.example" + gitExportRoutePrefix + remoteTaskID + ".git",
		gitExportRoutePrefix + remoteTaskID + ".git?access_token=secret",
		gitExportRoutePrefix + "00000000-0000-4000-8000-000000000029.git",
		gitExportRoutePrefix + remoteTaskID + ".git/../other.git",
		gitExportRoutePrefix + remoteTaskID + "%2egit",
	} {
		t.Run(gitURL, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				d := validRemoteDescriptor()
				d.GitURL = gitURL
				writeRemoteDescriptor(w, d)
			}))
			defer server.Close()
			_, err := ResolveRemote(context.Background(), server.Client(), server.URL, token, remoteTaskID)
			if err == nil || strings.Contains(err.Error(), token) {
				t.Fatalf("expected safe refusal of %q, got %v", gitURL, err)
			}
		})
	}
}

func TestResolveRemoteRejectsForeignTaskIdentityAndInvalidID(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d := validRemoteDescriptor()
		d.TaskInstanceID = "00000000-0000-4000-8000-000000000029"
		writeRemoteDescriptor(w, d)
	}))
	defer server.Close()
	if _, err := ResolveRemote(context.Background(), server.Client(), server.URL, "token", "not-a-uuid"); err == nil {
		t.Fatal("invalid TaskInstance ID was accepted")
	}
	if _, err := ResolveRemote(context.Background(), server.Client(), server.URL, "token", remoteTaskID); err == nil {
		t.Fatal("mismatched TaskInstance binding was accepted")
	}
}

func TestResolveRemoteRejectsUnknownFieldsAndMissingCacheFence(t *testing.T) {
	for _, caseName := range []string{"unknown-field", "missing-no-store", "wrong-schema", "malformed-owner-id"} {
		t.Run(caseName, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if caseName == "unknown-field" {
					encoded, _ := json.Marshal(validRemoteDescriptor())
					var unexpected map[string]any
					_ = json.Unmarshal(encoded, &unexpected)
					unexpected["private_tests"] = "secret"
					writeRemoteDescriptor(w, unexpected)
					return
				}
				if caseName == "missing-no-store" {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(validRemoteDescriptor())
					return
				}
				d := validRemoteDescriptor()
				if caseName == "wrong-schema" {
					d.Schema = "codechuman.checkout.v2"
				} else {
					d.CourseID = "not-a-uuid"
				}
				writeRemoteDescriptor(w, d)
			}))
			defer server.Close()
			if _, err := ResolveRemote(context.Background(), server.Client(), server.URL, "token", remoteTaskID); err == nil {
				t.Fatal("invalid remote descriptor was accepted")
			}
		})
	}
}
