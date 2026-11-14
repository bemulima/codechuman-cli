package checkout

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	checkoutRoute        = "/api/v1/ide/checkouts"
	gitExportRoutePrefix = "/api/v1/ide/git/exports/"
	maxDescriptorBytes   = 16 << 10
)

var canonicalTaskID = regexp.MustCompile("^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$")

// The live Practice descriptor deliberately provides a relative Git URL and
// identity fields only. Course materials must be fetched via their own
// authorized owner contract, never assumed to be part of this response.
type remoteDescriptor struct {
	Schema         string `json:"schema"`
	CourseID       string `json:"course_id"`
	CourseSlug     string `json:"course_slug"`
	LessonID       string `json:"lesson_id"`
	LessonNumber   int    `json:"lesson_number"`
	LessonRevision string `json:"lesson_revision"`
	TaskInstanceID string `json:"task_instance_id"`
	SandboxID      string `json:"sandbox_id"`
	GitURL         string `json:"git_url"`
}

// ResolveRemote obtains the owner-verified checkout descriptor from the
// authenticated Gateway route. The caller supplies only a TaskInstance ID.
// The bearer never enters a URL, a serialized manifest, or local storage.
func ResolveRemote(ctx context.Context, client *http.Client, baseURL, bearerToken, taskInstanceID string) (Spec, error) {
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" ||
		base.User != nil || base.RawQuery != "" || base.Fragment != "" || base.Opaque != "" ||
		(base.Path != "" && base.Path != "/") {
		return Spec{}, errors.New("Codechuman API base must be an HTTPS origin without credentials, path or query")
	}
	if !canonicalTaskID.MatchString(taskInstanceID) {
		return Spec{}, errors.New("task_instance_id must be a canonical UUID")
	}
	if len(bearerToken) == 0 || len(bearerToken) > 8192 ||
		strings.TrimSpace(bearerToken) != bearerToken ||
		strings.ContainsAny(bearerToken, "\r\n\x00\t ") {
		return Spec{}, errors.New("invalid bearer token")
	}
	requestJSON, err := json.Marshal(struct {
		TaskInstanceID string `json:"task_instance_id"`
	}{taskInstanceID})
	if err != nil {
		return Spec{}, err
	}
	origin := base.Scheme + "://" + base.Host
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+checkoutRoute, bytes.NewReader(requestJSON))
	if err != nil {
		return Spec{}, err
	}
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	// A bearer must never follow an attacker-controlled HTTPS redirect, even
	// when the caller provided a client with a redirect policy.
	safeClient := *client
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	if safeClient.Timeout == 0 {
		safeClient.Timeout = 12 * time.Second
	}
	response, err := safeClient.Do(req)
	if err != nil {
		return Spec{}, fmt.Errorf("checkout resolver request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Spec{}, fmt.Errorf("checkout resolver returned HTTP %d", response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return Spec{}, errors.New("checkout resolver returned a non-JSON response")
	}
	if !strings.Contains(strings.ToLower(response.Header.Get("Cache-Control")), "no-store") {
		return Spec{}, errors.New("checkout resolver must disable caching")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxDescriptorBytes+1))
	if err != nil || len(body) == 0 || len(body) > maxDescriptorBytes {
		return Spec{}, errors.New("checkout descriptor is unavailable or oversized")
	}
	var descriptor remoteDescriptor
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&descriptor); err != nil {
		return Spec{}, fmt.Errorf("invalid checkout descriptor: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return Spec{}, errors.New("checkout descriptor must contain exactly one JSON object")
	}
	if descriptor.Schema != schemaVersion || descriptor.TaskInstanceID != taskInstanceID {
		return Spec{}, errors.New("checkout TaskInstance binding differs from request")
	}
	if descriptor.GitURL != gitExportRoutePrefix+taskInstanceID+".git" {
		return Spec{}, errors.New("checkout Git URL must be the exact TaskInstance export path")
	}
	for _, id := range []string{descriptor.CourseID, descriptor.LessonID, descriptor.LessonRevision, descriptor.SandboxID} {
		if !canonicalTaskID.MatchString(id) {
			return Spec{}, errors.New("checkout descriptor contains an invalid owner identity")
		}
	}
	spec := Spec{
		Schema:         descriptor.Schema,
		CourseID:       descriptor.CourseID,
		CourseSlug:     descriptor.CourseSlug,
		LessonID:       descriptor.LessonID,
		LessonNumber:   descriptor.LessonNumber,
		LessonRevision: descriptor.LessonRevision,
		TaskInstanceID: descriptor.TaskInstanceID,
		SandboxID:      descriptor.SandboxID,
		CloneURL:       origin + descriptor.GitURL,
	}
	if err := spec.Validate(); err != nil {
		return Spec{}, fmt.Errorf("invalid checkout descriptor: %w", err)
	}
	return spec, nil
}
