/*
 * TencentBlueKing is pleased to support the open source community by making
 * 蓝鲸智云 - bk-cli (BlueKing - Cli) available.
 * Copyright (C) Tencent. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://opensource.org/licenses/MIT
 *
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * We undertake not to change the open source license (MIT license) applicable
 * to the current version of the project delivered to anyone in the future.
 */

package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	json "github.com/goccy/go-json"

	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

// pathPlaceholderRe matches {name} placeholders in URL path templates.
var pathPlaceholderRe = regexp.MustCompile(`\{([^}]+)\}`)

// PathValueValidator validates one placeholder value before substitution.
type PathValueValidator func(value string) error

// SubstitutePath replaces {name} placeholders in apiPath with values from
// pathJSON. It returns an error if pathJSON is invalid, if a placeholder has
// no matching key, or if pathJSON contains extra keys not present in apiPath.
func SubstitutePath(apiPath, pathJSON string) (string, error) {
	if pathJSON == "" {
		// No substitution requested; verify no placeholders remain.
		if m := pathPlaceholderRe.FindString(apiPath); m != "" {
			return "", fmt.Errorf(
				"unresolved placeholder %s in api_path; provide --path with a value for %q",
				m,
				m[1:len(m)-1],
			)
		}
		return apiPath, nil
	}

	vals, err := parsePathValues(pathJSON)
	if err != nil {
		return "", err
	}

	return SubstitutePathValues(apiPath, vals, map[string]PathValueValidator{
		"gateway_name": validate.ValidateGatewayName,
	})
}

// SubstitutePathValues replaces placeholders in apiPath using already parsed values.
func SubstitutePathValues(
	apiPath string,
	vals map[string]string,
	validators map[string]PathValueValidator,
) (string, error) {
	if len(vals) == 0 {
		if m := pathPlaceholderRe.FindString(apiPath); m != "" {
			return "", fmt.Errorf(
				"unresolved placeholder %s in api_path; provide --path with a value for %q",
				m,
				m[1:len(m)-1],
			)
		}
		return apiPath, nil
	}

	used := make(map[string]bool, len(vals))

	for key, rawValue := range vals {
		if validators == nil {
			continue
		}
		if validator, ok := validators[key]; ok && validator != nil {
			if err := validator(rawValue); err != nil {
				return "", fmt.Errorf("invalid --path value for %q: %w", key, err)
			}
		}
	}

	result := pathPlaceholderRe.ReplaceAllStringFunc(apiPath, func(match string) string {
		key := match[1 : len(match)-1]
		v, ok := vals[key]
		if !ok {
			return match // leave unresolved; checked below
		}
		used[key] = true
		return v
	})

	// Check for unresolved placeholders.
	if m := pathPlaceholderRe.FindString(result); m != "" {
		return "", fmt.Errorf(
			"unresolved placeholder %s in api_path; provide --path with a value for %q",
			m,
			m[1:len(m)-1],
		)
	}

	// Check for extra keys.
	for k := range vals {
		if !used[k] {
			return "", fmt.Errorf("--path key %q does not match any placeholder in api_path %q", k, apiPath)
		}
	}

	return result, nil
}

func parsePathValues(pathJSON string) (map[string]string, error) {
	var rawVals map[string]json.RawMessage
	if err := json.Unmarshal([]byte(pathJSON), &rawVals); err != nil {
		return nil, fmt.Errorf("invalid --path JSON: %w", err)
	}

	vals := make(map[string]string, len(rawVals))
	for key, raw := range rawVals {
		value, err := stringifyPathValue(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid --path value for %q: %w", key, err)
		}
		vals[key] = value
	}

	return vals, nil
}

func stringifyPathValue(raw json.RawMessage) (string, error) {
	raw = json.RawMessage(bytes.TrimSpace(raw))
	if len(raw) == 0 {
		return "", fmt.Errorf("empty JSON value")
	}

	switch raw[0] {
	case '"':
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		return url.PathEscape(value), nil
	case '{', '[':
		return "", fmt.Errorf("must be a string, number, or boolean")
	case 'n':
		return "", fmt.Errorf("must not be null")
	default:
		return url.PathEscape(string(raw)), nil
	}
}

// Request represents an API request to be built and executed.
type Request struct {
	Method     string
	URL        string
	ParamsJSON string            // JSON string for query params
	BodyJSON   string            // JSON string for request body
	Headers    map[string]string // Additional headers
	AuthHeader string            // X-Bkapi-Authorization value
	TenantID   string            // X-Bk-Tenant-Id value
}

// Build creates an http.Request from the API request spec.
func (r *Request) Build() (*http.Request, error) {
	var body io.Reader
	if r.BodyJSON != "" {
		if !json.Valid([]byte(r.BodyJSON)) {
			return nil, fmt.Errorf("invalid --body JSON: not valid JSON")
		}
		body = strings.NewReader(r.BodyJSON)
	}

	parsedURL, err := parseAndValidateRequestURL(r.URL)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(context.Background(), r.Method, parsedURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)

	// Set content type for requests with body
	if r.BodyJSON != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Set auth header
	if r.AuthHeader != "" {
		req.Header.Set("X-Bkapi-Authorization", r.AuthHeader)
	}

	// Set tenant header
	if r.TenantID != "" {
		req.Header.Set("X-Bk-Tenant-Id", r.TenantID)
	}

	// Apply custom headers last so explicit CLI --header values win.
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}

	// Add query params
	if err := AddQueryParams(req, r.ParamsJSON); err != nil {
		return nil, err
	}

	return req, nil
}

func parseAndValidateRequestURL(rawURL string) (*url.URL, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse request URL: %w", err)
	}
	if err := validateRequestURL(parsedURL); err != nil {
		return nil, err
	}
	return parsedURL, nil
}

func validateRequestURL(parsedURL *url.URL) error {
	if parsedURL == nil {
		return fmt.Errorf("request URL is required")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("request URL must use http or https")
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("request URL must include a host")
	}
	return nil
}

// ParseHeaderFlags parses --header "key:value" flag values into a map.
func ParseHeaderFlags(headerFlags []string) (map[string]string, error) {
	headers := make(map[string]string, len(headerFlags))
	for _, h := range headerFlags {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --header value %q: expected key:value", h)
		}

		key := strings.TrimSpace(parts[0])
		if key == "" {
			return nil, fmt.Errorf("invalid --header value %q: header name cannot be empty", h)
		}
		if err := validate.ValidateHeaderName(key); err != nil {
			return nil, fmt.Errorf("invalid --header value %q: %w", h, err)
		}

		value := strings.TrimSpace(parts[1])
		if err := validate.ValidateHeaderValue(value); err != nil {
			return nil, fmt.Errorf("invalid --header value %q: %w", h, err)
		}

		headers[key] = value
	}
	return headers, nil
}
