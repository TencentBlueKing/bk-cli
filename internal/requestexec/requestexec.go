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

// Package requestexec provides the shared request execution path used by raw API
// commands and system actions.
package requestexec

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/TencentBlueKing/bk-cli/internal/api"
	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

const (
	rawAPITimeoutErrorLabel = "--timeout value"
	actionTimeoutErrorLabel = "action timeout"
)

// Runtime contains resolved context state shared across one or more API calls.
type Runtime struct {
	ContextName string
	Config      *config.Config
	Credential  *credential.Credential
	DryRun      bool
	Verbose     bool
	Insecure    bool
}

// RuntimeOptions controls request execution behavior beyond the persistent
// context and output flags.
type RuntimeOptions struct {
	Insecure bool
}

// RequestSpec describes a single outbound API request.
type RequestSpec struct {
	GatewayName string
	Method      string
	Path        string
	ParamsJSON  string
	BodyJSON    string
	Headers     []string
	Stage       string
	Timeout     string
	AuthConfig  api.AuthPolicy
}

// RequestResult contains the envelope produced by a single API request.
type RequestResult struct {
	Envelope      *output.Envelope
	DryRunRequest *output.DryRunRequest
}

// ResolveRuntime loads the active context config for later request execution.
// Credentials are loaded lazily on the first outbound request so local-only
// callers can still reuse the shared runtime helpers.
func ResolveRuntime(ctxOverride string, dryRun, verbose bool) (*Runtime, error) {
	return ResolveRuntimeWithOptions(ctxOverride, dryRun, verbose, RuntimeOptions{})
}

// ResolveRuntimeWithOptions loads the active context config with request-level options.
func ResolveRuntimeWithOptions(
	ctxOverride string,
	dryRun, verbose bool,
	opts RuntimeOptions,
) (*Runtime, error) {
	ctxName, cfg, err := config.ResolveContext(ctxOverride)
	if err != nil {
		return nil, output.UserError("config_error", err.Error(),
			"Run: bk-cli context init --bk_api_url_tmpl=...")
	}

	return &Runtime{
		ContextName: ctxName,
		Config:      cfg,
		DryRun:      dryRun,
		Verbose:     verbose,
		Insecure:    opts.Insecure,
	}, nil
}

func loadCredential(ctxName string) (*credential.Credential, error) {
	credPath := config.CredentialsPath(ctxName)
	key, err := credential.DeriveKey()
	if err != nil {
		return nil, output.SystemError("crypto_error", err.Error(), "")
	}

	cred, err := credential.LoadFromFile(credPath, key)
	if err != nil {
		return nil, output.UserError("auth_required",
			fmt.Sprintf("No credentials found for context %q: %s", ctxName, err),
			"Run: bk-cli auth login")
	}

	return cred, nil
}

func ensureRuntimeCredential(runtime *Runtime) error {
	if runtime.Credential != nil {
		return nil
	}
	if runtime.ContextName == "" {
		return fmt.Errorf("runtime context name is required when credential is not preloaded")
	}

	cred, err := loadCredential(runtime.ContextName)
	if err != nil {
		return err
	}
	runtime.Credential = cred
	return nil
}

// ExecuteRequest builds, validates, and optionally executes a system action request without printing.
func ExecuteRequest(runtime *Runtime, spec RequestSpec) (*RequestResult, error) {
	if spec.AuthConfig == nil {
		return nil, fmt.Errorf("request authConfig is required")
	}
	return executeRequest(runtime, spec, actionTimeoutErrorLabel)
}

// ExecuteRawAPIRequest builds, validates, and optionally executes a raw API request without printing.
func ExecuteRawAPIRequest(runtime *Runtime, spec RequestSpec) (*RequestResult, error) {
	return executeRequest(runtime, spec, rawAPITimeoutErrorLabel)
}

func executeRequest(runtime *Runtime, spec RequestSpec, timeoutErrorLabel string) (*RequestResult, error) {
	if runtime == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	if runtime.Config == nil {
		return nil, fmt.Errorf("runtime config is required")
	}

	headerMap, err := api.ParseHeaderFlags(spec.Headers)
	if err != nil {
		return nil, output.UserError(
			"request_error",
			err.Error(),
			"Use repeatable --header key:value entries",
		)
	}
	if err := validateUserHeaders(headerMap, spec.BodyJSON); err != nil {
		return nil, output.UserError(
			"request_error",
			err.Error(),
			"Use repeatable --header key:value entries, and let bk-cli manage JSON content type when --body is provided",
		)
	}

	// here the `bk-cli api` authConfig is nil; the `bk-cli action` authConfig is not nil
	// would ensure the cred is loaded
	if spec.AuthConfig == nil || spec.AuthConfig.RequiresAuth() {
		if err := ensureRuntimeCredential(runtime); err != nil {
			return nil, err
		}
	}

	timeout, err := resolveTimeout(runtime.Config.Timeout, spec.Timeout, timeoutErrorLabel)
	if err != nil {
		return nil, output.UserError(
			"request_error",
			err.Error(),
			"Use a valid duration such as 60s or 2m",
		)
	}
	gatewayName := config.ResolveGatewayName(runtime.Config.BkAPIURLTmpl, spec.GatewayName)

	if err := validate.ValidateGatewayName(gatewayName); err != nil {
		return nil, output.UserError(
			"invalid_gateway_name",
			err.Error(),
			"Use a gateway name matching ^[a-z][a-z0-9-]{2,29}$",
		)
	}

	fullURL, err := api.BuildURL(runtime.Config.BkAPIURLTmpl, gatewayName, spec.Stage, spec.Path)
	if err != nil {
		return nil, output.UserError("url_error", err.Error(), "Check bk_api_url_tmpl in config")
	}

	authHeader, err := api.BuildAuthHeader(runtime.Credential, spec.AuthConfig)
	if err != nil {
		return nil, output.SystemError("auth_header_error", err.Error(), "")
	}

	reqSpec := &api.Request{
		Method:     spec.Method,
		URL:        fullURL,
		ParamsJSON: spec.ParamsJSON,
		BodyJSON:   spec.BodyJSON,
		Headers:    headerMap,
		AuthHeader: authHeader,
		TenantID:   resolveTenantID(runtime.Config),
	}

	httpReq, err := reqSpec.Build()
	if err != nil {
		return nil, output.UserError(
			"request_error",
			err.Error(),
			"Check generated flags, --body JSON, and --header values",
		)
	}

	if runtime.DryRun {
		env := api.BuildDryRunEnvelope(reqSpec)
		return &RequestResult{
			Envelope:      env,
			DryRunRequest: env.Request,
		}, nil
	}

	if runtime.Verbose {
		logVerboseRequest(httpReq)
	}

	client := api.NewClient(timeout, api.WithInsecureSkipVerify(runtime.Insecure))
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, output.SystemError("network_error",
			fmt.Sprintf("Request failed: %s", err),
			"Check network connectivity and VPN")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if runtime.Verbose {
		logVerboseResponse(resp)
	}

	envelope, err := api.BuildResponseEnvelope(resp)
	if err != nil {
		return nil, output.SystemError("response_error", err.Error(), "")
	}

	return &RequestResult{Envelope: envelope}, nil
}

func validateUserHeaders(headers map[string]string, bodyJSON string) error {
	for key := range headers {
		if bodyJSON != "" && strings.EqualFold(key, "Content-Type") {
			return fmt.Errorf("header %q cannot be overridden when --body is provided", "Content-Type")
		}
	}

	return nil
}

func resolveTenantID(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.TenantID
}

func resolveTimeout(defaultTimeout time.Duration, override, timeoutErrorLabel string) (time.Duration, error) {
	if override == "" {
		if defaultTimeout == 0 {
			return config.DefaultTimeout, nil
		}
		return defaultTimeout, nil
	}

	if timeoutErrorLabel == "" {
		timeoutErrorLabel = actionTimeoutErrorLabel
	}

	timeout, err := time.ParseDuration(override)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", timeoutErrorLabel, override, err)
	}
	if timeout <= 0 {
		return 0, fmt.Errorf("invalid %s %q: must be greater than 0", timeoutErrorLabel, override)
	}
	return timeout, nil
}

func logVerboseRequest(httpReq *http.Request) {
	fmt.Fprintf(os.Stderr, "> %s %s\n", httpReq.Method, httpReq.URL.String())
	for key, values := range httpReq.Header {
		if strings.EqualFold(key, "X-Bkapi-Authorization") {
			fmt.Fprintf(os.Stderr, "> %s: {redacted}\n", key)
			continue
		}
		fmt.Fprintf(os.Stderr, "> %s: %s\n", key, strings.Join(values, ", "))
	}
	fmt.Fprintln(os.Stderr)
}

func logVerboseResponse(resp *http.Response) {
	fmt.Fprintf(os.Stderr, "< %d %s\n", resp.StatusCode, resp.Status)
	for key, values := range resp.Header {
		fmt.Fprintf(os.Stderr, "< %s: %s\n", key, strings.Join(values, ", "))
	}
	fmt.Fprintln(os.Stderr)
}
