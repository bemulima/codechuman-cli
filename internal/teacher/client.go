// Package teacher connects the IDE to existing owner APIs; Dialog owns history.
package teacher

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var uuid = regexp.MustCompile(`^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$`)

type Request struct {
	SyncMain bool   `json:"sync_main"`
	Token    string `json:"token"`
	Body     string `json:"body"`
	Key      string `json:"idempotency_key"`
}
type Message struct {
	ID     string  `json:"id"`
	Author string  `json:"author_type"`
	Body   string  `json:"body"`
	Reply  *string `json:"reply_to_message_id"`
}
type Result struct {
	DialogID string    `json:"dialog_id"`
	TaskID   string    `json:"task_instance_id"`
	Messages []Message `json:"messages"`
	Pending  bool      `json:"pending"`
}
type client struct {
	http          *http.Client
	origin, token string
}

func (c client) call(ctx context.Context, method, path string, payload any, out any) error {
	return c.callHeaders(ctx, method, path, payload, out, nil)
}
func (c client) callHeaders(ctx context.Context, method, path string, payload any, out any, headers map[string]string) error {
	var b bytes.Buffer
	if payload != nil {
		if e := json.NewEncoder(&b).Encode(payload); e != nil {
			return e
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, c.origin+path, &b)
	if e != nil {
		return errors.New("invalid Teacher request")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, e := c.http.Do(req)
	if e != nil {
		return errors.New("Teacher connection unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("Teacher API returned HTTP %d", res.StatusCode)
	}
	mt, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if mt != "application/json" {
		return errors.New("invalid Teacher response type")
	}
	raw, e := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if e != nil || len(raw) > 2<<20 {
		return errors.New("oversized Teacher response")
	}
	if json.Unmarshal(raw, out) != nil {
		return errors.New("invalid Teacher response")
	}
	return nil
}
func NewKey() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return ""
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func Chat(ctx context.Context, h *http.Client, origin, workspace string, input Request) (Result, error) {
	fail := Result{}
	u, e := url.Parse(origin)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return fail, errors.New("Teacher requires an HTTPS origin")
	}
	if input.Token == "" || len(input.Token) > 8192 || strings.ContainsAny(input.Token, " \t\n\r\x00") {
		return fail, errors.New("invalid Student token")
	}
	if len(input.Body) > 12000 {
		return fail, errors.New("Teacher message too long")
	}
	if input.Body != "" && !uuid.MatchString(input.Key) {
		return fail, errors.New("message idempotency key required")
	}
	p := filepath.Join(workspace, ".learning", "manifest.json")
	st, e := os.Lstat(p)
	if e != nil || !st.Mode().IsRegular() || st.Size() > 16384 {
		return fail, errors.New("safe checkout manifest required")
	}
	raw, e := os.ReadFile(p)
	if e != nil {
		return fail, errors.New("checkout manifest unreadable")
	}
	var m struct {
		Schema string `json:"schema"`
		Task   string `json:"task_instance_id"`
	}
	if json.Unmarshal(raw, &m) != nil || m.Schema != "codechuman.checkout.v1" || !uuid.MatchString(m.Task) {
		return fail, errors.New("invalid checkout identity")
	}
	if h == nil {
		h = &http.Client{Timeout: 12 * time.Second}
	}
	hc := *h
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if hc.Timeout == 0 {
		hc.Timeout = 12 * time.Second
	}
	c := client{&hc, strings.TrimRight(origin, "/"), input.Token}
	// Runtime rechecks actual Student ownership. No identity/action comes from local config.
	var envelope struct {
		Instance struct {
			ID        string `json:"id"`
			Action    string `json:"learning_action_id"`
			Status    string `json:"status"`
			Workspace struct {
				Schema    string `json:"schema"`
				SandboxID string `json:"sandbox_id"`
				Available bool   `json:"available"`
			} `json:"workspace_binding"`
		} `json:"instance"`
	}
	if e = c.call(ctx, "GET", "/api/practice-task/v2/task-instances/"+m.Task, nil, &envelope); e != nil {
		return fail, e
	}
	instance := envelope.Instance
	if instance.ID != m.Task || !uuid.MatchString(instance.Action) {
		return fail, errors.New("authorized TaskInstance context unavailable")
	}
	if input.SyncMain && strings.TrimSpace(input.Body) != "" {
		b := instance.Workspace
		if b.Schema != "practice-workspace-binding.v2" || !b.Available || !uuid.MatchString(b.SandboxID) {
			return fail, errors.New("real Sandbox binding required for saved code")
		}
		if e = c.syncMain(ctx, workspace, b.SandboxID, input.Key); e != nil {
			return fail, e
		}
	}
	var binding struct {
		ID        string `json:"id"`
		Context   string `json:"context_type"`
		ContextID string `json:"context_id"`
	}
	if e = c.call(ctx, "PUT", "/api/teacher/v1/teacher-dialog/ensure", map[string]string{"context_type": "practice_task", "context_id": m.Task}, &binding); e != nil {
		return fail, e
	}
	if !uuid.MatchString(binding.ID) || binding.Context != "practice_task" || binding.ContextID != m.Task {
		return fail, errors.New("Teacher context identity mismatch")
	}
	result := Result{DialogID: binding.ID, TaskID: m.Task}
	source := ""
	if strings.TrimSpace(input.Body) != "" {
		var msg Message
		e = c.call(ctx, "POST", "/api/dialog/v1/message/create", map[string]any{"dialog_id": binding.ID, "body": input.Body, "idempotency_key": input.Key, "learning_action_id": instance.Action}, &msg)
		if e != nil {
			return fail, e
		}
		if !uuid.MatchString(msg.ID) {
			return fail, errors.New("invalid committed message")
		}
		source = msg.ID
	}
	for {
		var page struct {
			Items []Message `json:"items"`
		}
		if e = c.call(ctx, "GET", "/api/dialog/v1/message/list?dialog_id="+binding.ID+"&limit=100", nil, &page); e != nil {
			return fail, e
		}
		result.Messages = page.Items
		if source == "" {
			return result, nil
		}
		for _, msg := range page.Items {
			if msg.Author == "personal_teacher" && msg.Reply != nil && *msg.Reply == source {
				return result, nil
			}
		}
		select {
		case <-ctx.Done():
			result.Pending = true
			return result, nil
		case <-time.After(time.Second):
		}
	}
}
