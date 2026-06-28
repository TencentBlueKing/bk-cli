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

package cmd

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/api"
	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

type doctorOptions struct {
	offline bool
	gateway string
	stage   string
	timeout time.Duration
}

type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type doctorCredential struct {
	Type        string `json:"type"`
	BkAppCode   string `json:"bk_app_code,omitempty"`
	BkAppSecret string `json:"bk_app_secret,omitempty"`
	BkToken     string `json:"bk_token,omitempty"`
	BkTicket    string `json:"bk_ticket,omitempty"`
	AccessToken string `json:"access_token,omitempty"`
	UserKey     string `json:"user_key,omitempty"`
}

type doctorContext struct {
	Name            string            `json:"name"`
	Active          bool              `json:"active"`
	Selected        bool              `json:"selected"`
	HasConfig       bool              `json:"has_config"`
	ConfigError     string            `json:"config_error,omitempty"`
	BkAPIURLTmpl    string            `json:"bk_api_url_tmpl,omitempty"`
	BkAuthURL       string            `json:"bk_auth_url,omitempty"`
	TenantID        string            `json:"tenant_id,omitempty"`
	UserKey         string            `json:"user_key,omitempty"`
	Timeout         string            `json:"timeout,omitempty"`
	RenderedURL     string            `json:"rendered_url,omitempty"`
	HasCredentials  bool              `json:"has_credentials"`
	Credential      *doctorCredential `json:"credential,omitempty"`
	CredentialError string            `json:"credential_error,omitempty"`
}

func newDoctorCmd(contextGetter func() string, insecureGetter func() bool) *cobra.Command {
	opts := doctorOptions{
		gateway: "bk-apigateway",
		stage:   config.DefaultStage,
		timeout: 10 * time.Second,
	}

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check local context, credentials, URL rendering, and connectivity",
		Long: `Check whether bk-cli is ready to call BlueKing API gateways.

The command reports local contexts, the selected context, masked credentials,
the URL rendered from bk_api_url_tmpl, and a lightweight connectivity probe.

Examples:
  bk-cli doctor
  bk-cli doctor --offline
  bk-cli doctor --context dev --gateway bk-iam`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.ValidateGatewayName(opts.gateway); err != nil {
				return output.UserError(
					"invalid_gateway_name",
					err.Error(),
					"Use a gateway name like bk-apigateway",
				)
			}
			return runDoctor(cmd, opts, contextGetter(), insecureGetter())
		},
	}

	cmd.Flags().BoolVar(&opts.offline, "offline", false, "Skip network checks and only inspect local state")
	cmd.Flags().StringVar(&opts.gateway, "gateway", "bk-apigateway", "Gateway name used to render bk_api_url_tmpl")
	cmd.Flags().StringVar(
		&opts.stage,
		"stage",
		config.DefaultStage,
		"API gateway stage used to render bk_api_url_tmpl",
	)
	cmd.Flags().DurationVar(&opts.timeout, "timeout", 10*time.Second, "Connectivity probe timeout")

	return cmd
}

func runDoctor(cmd *cobra.Command, opts doctorOptions, contextOverride string, insecure bool) error {
	names, listErr := config.ListContexts()
	sort.Strings(names)

	activeContext, activeErr := config.ActiveContextName()
	selectedContext, selectedCfg, resolveErr := config.ResolveContextReadOnly(contextOverride)

	checks := []doctorCheck{
		passDoctorCheck("cli_version", buildInfo.Version),
	}

	switch {
	case listErr != nil:
		checks = append(
			checks,
			failDoctorCheck(
				"contexts",
				listErr.Error(),
				"Check the BK_CLI_CONFIG_DIR or ~/.bk-cli permissions",
			),
		)
	case len(names) == 0:
		checks = append(checks, failDoctorCheck(
			"contexts",
			"no context configured",
			"Run: bk-cli context init --bk_api_url_tmpl=URL",
		))
	default:
		checks = append(checks, passDoctorCheck("contexts", fmt.Sprintf("%d context(s) found", len(names))))
	}

	switch {
	case activeErr != nil:
		checks = append(checks, failDoctorCheck("active_context", activeErr.Error(), "Check ~/.bk-cli/current"))
	case activeContext == "":
		checks = append(
			checks,
			warnDoctorCheck(
				"active_context",
				"no active context marker",
				"Run: bk-cli context use CONTEXT",
			),
		)
	default:
		checks = append(checks, passDoctorCheck("active_context", activeContext))
	}

	switch {
	case resolveErr != nil:
		checks = append(
			checks,
			failDoctorCheck("selected_context", resolveErr.Error(), "Run: bk-cli context list"),
		)
	case selectedContext == "":
		checks = append(
			checks,
			failDoctorCheck(
				"selected_context",
				"no context selected",
				"Run: bk-cli context init --bk_api_url_tmpl=URL",
			),
		)
	default:
		checks = append(checks, passDoctorCheck("selected_context", selectedContext))
	}

	contexts := make([]doctorContext, 0, len(names))
	for _, name := range names {
		contexts = append(
			contexts,
			inspectDoctorContext(name, activeContext, selectedContext, opts.gateway, opts.stage),
		)
	}

	selectedRenderedURL := ""
	if selectedCfg != nil {
		url, err := renderDoctorURL(selectedCfg, opts.gateway, opts.stage)
		if err != nil {
			checks = append(
				checks,
				failDoctorCheck(
					"url_template",
					err.Error(),
					"Fix bk_api_url_tmpl in the selected context config.yaml",
				),
			)
		} else {
			selectedRenderedURL = url
			checks = append(checks, passDoctorCheck("url_template", url))
		}
	}

	selectedReport := findDoctorContextReport(contexts, selectedContext)
	switch {
	case selectedReport == nil:
		if selectedContext != "" && resolveErr == nil {
			checks = append(
				checks,
				failDoctorCheck(
					"selected_credentials",
					"selected context was not found in context list",
					"Run: bk-cli context list",
				),
			)
		}
	case selectedReport.CredentialError != "":
		checks = append(
			checks,
			failDoctorCheck(
				"selected_credentials",
				selectedReport.CredentialError,
				"Run: bk-cli auth login",
			),
		)
	case !selectedReport.HasCredentials:
		checks = append(checks, failDoctorCheck(
			"selected_credentials",
			fmt.Sprintf("context %q has no stored credentials", selectedContext),
			"Run: bk-cli auth login",
		))
	default:
		checks = append(checks, passDoctorCheck("selected_credentials", selectedReport.Credential.Type))
	}

	checks = append(checks, checkDoctorConnectivity(cmd.Context(), opts, insecure, selectedRenderedURL))

	ok := doctorChecksOK(checks)
	data := map[string]any{
		"ok":               ok,
		"version":          buildInfo.Version,
		"config_dir":       config.BaseDirectory(),
		"context_override": contextOverride,
		"active_context":   activeContext,
		"selected_context": selectedContext,
		"probe_gateway":    opts.gateway,
		"probe_stage":      opts.stage,
		"contexts":         contexts,
		"checks":           checks,
	}

	if err := (&output.Envelope{OK: ok, Data: data}).WriteJSON(cmd.OutOrStdout()); err != nil {
		return err
	}
	if !ok {
		return &output.CLIError{
			ExitCode: output.ExitCodeUserError,
			Code:     "doctor_failed",
			Message:  "one or more doctor checks failed",
		}
	}
	return nil
}

func inspectDoctorContext(name, activeContext, selectedContext, gateway, stage string) doctorContext {
	report := doctorContext{
		Name:     name,
		Active:   name == activeContext,
		Selected: name == selectedContext,
	}

	cfg, err := config.Load(config.ConfigPath(name))
	if err != nil {
		report.ConfigError = err.Error()
		return report
	}

	report.HasConfig = true
	report.BkAPIURLTmpl = cfg.BkAPIURLTmpl
	report.BkAuthURL = cfg.BkAuthURL
	report.TenantID = cfg.TenantID
	report.UserKey = cfg.UserKey
	report.Timeout = cfg.Timeout.String()

	if rendered, err := renderDoctorURL(cfg, gateway, stage); err == nil {
		report.RenderedURL = rendered
	} else {
		report.ConfigError = err.Error()
	}

	cred, hasCredentials, err := loadDoctorCredential(name)
	report.HasCredentials = hasCredentials
	if err != nil {
		report.CredentialError = err.Error()
		return report
	}
	report.Credential = cred
	return report
}

func renderDoctorURL(cfg *config.Config, gateway, stage string) (string, error) {
	if err := config.ValidateURLTemplate(cfg.BkAPIURLTmpl); err != nil {
		return "", err
	}
	effectiveGateway := config.ResolveGatewayName(cfg.BkAPIURLTmpl, gateway)
	return api.BuildURL(cfg.BkAPIURLTmpl, effectiveGateway, stage, "")
}

func loadDoctorCredential(ctxName string) (*doctorCredential, bool, error) {
	credPath := config.CredentialsPath(ctxName)
	if !credential.Exists(credPath) {
		return nil, false, nil
	}

	key, err := credential.DeriveKey()
	if err != nil {
		return nil, true, err
	}

	cred, err := credential.LoadFromFile(credPath, key)
	if err != nil {
		return nil, true, err
	}

	report := &doctorCredential{Type: string(cred.Type)}
	switch cred.Type {
	case credential.TypeAppUser:
		report.BkAppCode = maskDoctorSecret(cred.BkAppCode)
		report.BkAppSecret = maskDoctorSecret(cred.BkAppSecret)
		report.BkToken = maskDoctorSecret(cred.BkToken)
		report.BkTicket = maskDoctorSecret(cred.BkTicket)
		report.UserKey = cred.UserKeyType()
	case credential.TypeAccessToken:
		report.AccessToken = maskDoctorSecret(cred.AccessToken)
	default:
		return nil, true, fmt.Errorf("unknown credential type: %q", cred.Type)
	}

	return report, true, nil
}

func maskDoctorSecret(value string) string {
	if value == "" {
		return ""
	}

	runes := []rune(value)
	switch {
	case len(runes) <= 2:
		return strings.Repeat("*", len(runes))
	case len(runes) <= 4:
		return string(runes[:1]) + "***" + string(runes[len(runes)-1:])
	case len(runes) <= 8:
		return string(runes[:2]) + "***" + string(runes[len(runes)-2:])
	default:
		return string(runes[:4]) + "***" + string(runes[len(runes)-4:])
	}
}

func checkDoctorConnectivity(ctx context.Context, opts doctorOptions, insecure bool, renderedURL string) doctorCheck {
	if opts.offline {
		return skipDoctorCheck("connectivity", "skipped (--offline)")
	}
	if renderedURL == "" {
		return failDoctorCheck(
			"connectivity",
			"no rendered URL available for selected context",
			"Fix selected context first",
		)
	}
	if opts.timeout <= 0 {
		return failDoctorCheck(
			"connectivity",
			"--timeout must be greater than 0",
			"Use a duration like 5s or 30s",
		)
	}

	client := api.NewClient(opts.timeout, api.WithInsecureSkipVerify(insecure))
	if err := probeDoctorURL(ctx, client.HTTPClient, renderedURL); err != nil {
		return failDoctorCheck(
			"connectivity",
			fmt.Sprintf("%s unreachable: %s", renderedURL, err),
			"Check network, DNS, proxy, TLS, or use --insecure for local certificate issues",
		)
	}
	return passDoctorCheck("connectivity", renderedURL+" reachable")
}

func probeDoctorURL(ctx context.Context, client *http.Client, renderedURL string) error {
	ctx, cancel := context.WithTimeout(ctx, client.Timeout)
	defer cancel()

	statusCode, err := probeDoctorURLWithMethod(ctx, client, http.MethodHead, renderedURL)
	if err != nil {
		return err
	}
	if statusCode == http.StatusMethodNotAllowed {
		_, err = probeDoctorURLWithMethod(ctx, client, http.MethodGet, renderedURL)
		return err
	}
	return nil
}

func probeDoctorURLWithMethod(ctx context.Context, client *http.Client, method, renderedURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, renderedURL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, nil
}

func findDoctorContextReport(contexts []doctorContext, name string) *doctorContext {
	for i := range contexts {
		if contexts[i].Name == name {
			return &contexts[i]
		}
	}
	return nil
}

func doctorChecksOK(checks []doctorCheck) bool {
	for _, check := range checks {
		if check.Status == "fail" {
			return false
		}
	}
	return true
}

func passDoctorCheck(name, message string) doctorCheck {
	return doctorCheck{Name: name, Status: "pass", Message: message}
}

func warnDoctorCheck(name, message, hint string) doctorCheck {
	return doctorCheck{Name: name, Status: "warn", Message: message, Hint: hint}
}

func failDoctorCheck(name, message, hint string) doctorCheck {
	return doctorCheck{Name: name, Status: "fail", Message: message, Hint: hint}
}

func skipDoctorCheck(name, message string) doctorCheck {
	return doctorCheck{Name: name, Status: "skip", Message: message}
}
