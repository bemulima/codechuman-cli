package checkout

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	checkoutRequestSchema = "codechuman.checkout-request.v1"
	checkoutRoute         = "/api/v1/ide/checkouts"
	gitExportRoutePrefix  = "/api/v1/ide/git/exports/"
)

var canonicalTaskID = regexp.MustCompile("^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$")

// ResolveRemote obtains the pinned checkout instructions from a trusted,
// same-origin Codechuman gateway. The endpoint is a NEW, proposed server
// contract and is not yet deployed. No token is written to disk or the URL.
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
		Schema         string `json:"schema"`
		TaskInstanceID string `json:"task_instance_id"`
	}{checkoutRequestSchema, taskInstanceID})
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
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return Spec{}, fmt.Errorf("checkout resolver returned HTTP %d", response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return Spec{}, errors.New("checkout resolver returned a non-JSON response")
	}
	spec, err := ReadSpec(response.Body)
	if err != nil {
		return Spec{}, fmt.Errorf("invalid server checkout specification: %w", err)
	}
	if spec.TaskInstanceID != taskInstanceID {
		return Spec{}, errors.New("checkout TaskInstance binding differs from request")
	}
	gitURL, err := url.Parse(spec.CloneURL)
	if err != nil || gitURL.Scheme != base.Scheme ||
		!strings.EqualFold(gitURL.Host, base.Host) ||
		!strings.HasPrefix(gitURL.Path, gitExportRoutePrefix) ||
		!strings.HasSuffix(gitURL.Path, ".git") {
		return Spec{}, errors.New("checkout Git HTTPS URL must use the authorized gateway origin and export route")
	}
	return spec, nil
}
