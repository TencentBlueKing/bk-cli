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
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	json "github.com/goccy/go-json"

	"github.com/TencentBlueKing/bk-cli/internal/config"
)

// Client handles HTTP communication with BlueKing API gateways.
type Client struct {
	HTTPClient *http.Client
}

type clientOptions struct {
	insecureSkipVerify bool
}

// ClientOption customizes HTTP client construction.
type ClientOption func(*clientOptions)

// WithInsecureSkipVerify skips HTTPS certificate verification when enabled.
func WithInsecureSkipVerify(enabled bool) ClientOption {
	return func(opts *clientOptions) {
		opts.insecureSkipVerify = enabled
	}
}

const defaultUserAgentVersion = "dev"

var userAgent = buildUserAgent(defaultUserAgentVersion)

// SetUserAgentVersion updates the default User-Agent used for bk-cli requests.
func SetUserAgentVersion(version string) {
	userAgent = buildUserAgent(version)
}

func buildUserAgent(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		version = defaultUserAgentVersion
	}
	return "bk-cli/" + version
}

// NewClient creates a new API client.
func NewClient(timeout time.Duration, opts ...ClientOption) *Client {
	if timeout == 0 {
		timeout = config.DefaultTimeout
	}

	options := clientOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	httpClient := &http.Client{
		Timeout: timeout,
	}
	if options.insecureSkipVerify {
		httpClient.Transport = insecureSkipVerifyTransport()
	}

	return &Client{
		HTTPClient: httpClient,
	}
}

func insecureSkipVerifyTransport() *http.Transport {
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		baseTransport = &http.Transport{}
	}
	transport := baseTransport.Clone()
	transport.TLSClientConfig = cloneTLSConfig(transport.TLSClientConfig)
	transport.TLSClientConfig.InsecureSkipVerify = true //nolint:gosec // Explicit --insecure behavior mirrors curl.
	return transport
}

func cloneTLSConfig(cfg *tls.Config) *tls.Config {
	if cfg == nil {
		return &tls.Config{}
	}
	return cfg.Clone()
}

// BuildURL constructs the full API URL from template, gateway, stage, and path.
//
// Template patterns:
//   - Path-based:      https://bkapi.example.com/api/{gateway_name}/
//   - Subdomain-based: https://{gateway_name}.example.com
//
// URL construction: render template → append /stage → append path
func BuildURL(tmpl, gatewayName, stage, apiPath string) (string, error) {
	if tmpl == "" {
		return "", fmt.Errorf("bk_api_url_tmpl is not configured")
	}
	if gatewayName == "" {
		return "", fmt.Errorf("gateway/system name is required")
	}

	// Render template with gateway name
	base := strings.ReplaceAll(tmpl, "{gateway_name}", gatewayName)

	// Ensure base has no trailing slash for clean joining
	base = strings.TrimRight(base, "/")

	// Append stage
	if stage == "" {
		stage = "prod"
	}
	staged := base + "/" + stage

	// Append API path
	apiPath = strings.TrimLeft(apiPath, "/")
	if apiPath == "" {
		return staged + "/", nil
	}

	return staged + "/" + apiPath, nil
}

// Do executes an HTTP request and returns the raw response.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if err := validateRequestURL(req.URL); err != nil {
		return nil, err
	}

	//nolint:gosec // Request URLs are validated before dispatch and are required for CLI API calls.
	return c.HTTPClient.Do(req)
}

// ParseResponse reads and parses the HTTP response body.
func ParseResponse(resp *http.Response) (any, error) {
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if len(body) == 0 {
		return nil, nil
	}

	// Try to parse as JSON
	var result any
	if err := json.Unmarshal(body, &result); err != nil {
		// Not JSON — return as string
		return string(body), nil
	}
	return result, nil
}

var responseDebugHeaders = map[string]struct{}{
	"content-type":   {},
	"content-length": {},
	"traceparent":    {},
}

// ExtractGatewayHeaders extracts trace headers and includes basic debug headers
// only when they add diagnostic value.
func ExtractGatewayHeaders(resp *http.Response) map[string]string {
	headers := make(map[string]string)
	if resp == nil {
		return headers
	}

	for _, key := range sortedHeaderKeys(resp.Header) {
		values := resp.Header.Values(key)
		if len(values) == 0 {
			continue
		}

		lower := strings.ToLower(key)
		if lower == "x-request-id" || strings.HasPrefix(lower, "x-bkapi-") {
			headers[key] = values[0]
			continue
		}

		if shouldIncludeResponseDebugHeader(resp, lower, values[0]) {
			headers[key] = values[0]
		}
	}

	return headers
}

// Keep the default envelope compact: trace headers are always included, while
// generic HTTP headers only show up when they explain unusual behavior.
func shouldIncludeResponseDebugHeader(resp *http.Response, key, value string) bool {
	if _, ok := responseDebugHeaders[key]; !ok {
		return false
	}

	// Error responses are the primary debug path, so include the allowlisted
	// HTTP headers to avoid requiring a second verbose-only reproduction.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return true
	}

	switch key {
	case "content-type":
		// Successful JSON is the normal case and adds little value; keep the
		// header only when the server returns a different payload type.
		return value != "" && !isJSONContentType(value)
	case "content-length":
		// Length is useful when the response is unexpectedly empty or when the
		// payload type itself is unusual enough to include Content-Type above.
		return value != "" &&
			(value == "0" || shouldIncludeResponseDebugHeader(resp, "content-type", resp.Header.Get("Content-Type")))
	case "traceparent":
		// W3C trace context is most valuable on failures, where the default
		// envelope should still let operators correlate the request upstream.
		return false
	default:
		return false
	}
}

func isJSONContentType(value string) bool {
	base := strings.TrimSpace(strings.SplitN(value, ";", 2)[0])
	return strings.EqualFold(base, "application/json") || strings.HasSuffix(strings.ToLower(base), "+json")
}

func sortedHeaderKeys(header http.Header) []string {
	keys := make([]string, 0, len(header))
	for key := range header {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// AddQueryParams adds JSON-parsed params as URL query parameters.
func AddQueryParams(req *http.Request, paramsJSON string) error {
	if paramsJSON == "" {
		return nil
	}

	params, err := parseQueryValues(paramsJSON)
	if err != nil {
		return fmt.Errorf("invalid --query JSON: %w", err)
	}

	q := req.URL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()

	return nil
}

func parseQueryValues(paramsJSON string) (map[string]string, error) {
	var rawParams map[string]json.RawMessage
	if err := json.Unmarshal([]byte(paramsJSON), &rawParams); err != nil {
		return nil, err
	}

	params := make(map[string]string, len(rawParams))
	for key, raw := range rawParams {
		value, err := stringifyQueryValue(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid query value for %q: %w", key, err)
		}
		params[key] = value
	}

	return params, nil
}

func stringifyQueryValue(raw json.RawMessage) (string, error) {
	raw = json.RawMessage(bytes.TrimSpace(raw))
	if len(raw) == 0 {
		return "", fmt.Errorf("empty JSON value")
	}

	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		return value, nil
	}

	return string(raw), nil
}
