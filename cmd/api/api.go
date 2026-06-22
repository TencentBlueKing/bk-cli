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

// Package api defines the raw API Cobra command.
package api

import (
	"errors"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/api"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	"github.com/TencentBlueKing/bk-cli/internal/requestexec"
	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

// NewAPICmd creates the api subcommand.
func NewAPICmd(
	getContext func() string,
	isDryRun, isVerbose, isInsecure func() bool,
) *cobra.Command {
	var (
		flagPath    string
		flagQuery   string
		flagBody    string
		flagHeaders []string
		flagStage   string
		flagTimeout string
	)

	cmd := &cobra.Command{
		Use:   "api <gateway_name> <method> <api_path> [flags]",
		Short: "Make raw API calls to BlueKing API gateways",
		Long: `Make direct HTTP calls to any BlueKing API gateway.

The CLI builds the full URL from the configured URL template, attaches
authentication headers, executes the request, and returns a JSON envelope.

URL construction:
  1. Render bk_api_url_tmpl with gateway_name
  2. Append /{stage} (default: prod)
  3. Substitute --path placeholders in api_path
  4. Append resolved api_path

Request rules:
  - --path values are escaped as single URL path segments
  - --header X-Bkapi-Authorization / X-Bk-Tenant-Id override the generated values
  - Content-Type is managed by bk-cli when --body is provided

403 guidance:
  - API gateway 403: if X-Bkapi-Error-Code is 1640301 or the message says
    "App has no permission", ask to apply API permission:
    bk_app_code - gateway_name - api_name/method/url
  - Business system 403: if the upstream body returns a business code such as
    bk_error_code 9900403 (IAM permission error), ask to apply business
    permission instead of API permission

Examples:
  # GET request
  bk-cli api bk-apigateway GET /api/v2/open/gateways/

  # GET with query params
  bk-cli api bk-apigateway GET /api/v2/open/gateways/ --query '{"name":"bk-iam","fuzzy":true}'

  # Path with value rendered directly (recommended for agents)
  bk-cli api bk-apigateway GET /api/v2/open/gateways/bk-iam/resources/

  # Path substitution via --path (when using templates)
  bk-cli api bk-apigateway GET /api/v2/open/gateways/{gateway_name}/resources/ \
    --path '{"gateway_name":"bk-iam"}'

  # POST with body
  bk-cli api bk-demo POST /api/v2/foo/ --body '{"name":"bar"}'

  # Custom headers
  bk-cli api bk-demo GET /api/v2/foo/ --header "X-Custom:value"

  # Intentionally override auth / tenant headers for this request
  bk-cli api bk-demo GET /api/v2/foo/ \
    --header 'X-Bkapi-Authorization:{"access_token":"custom-token"}' \
    --header 'X-Bk-Tenant-Id:tenant-b'

  # Use testing stage
  bk-cli api bk-demo GET /api/v2/foo/ --stage testing

  # Override context timeout for this request
  bk-cli api bk-demo GET /api/v2/foo/ --timeout 180s

  # Preview request without executing
  bk-cli api bk-demo GET /api/v2/foo/ --dry-run`,
		Args: cobra.MinimumNArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			gatewayName := args[0]
			method := strings.ToUpper(args[1])
			apiPath := args[2]

			return runAPI(
				cmd.OutOrStdout(),
				gatewayName,
				method,
				apiPath,
				flagPath,
				flagQuery,
				flagBody,
				flagHeaders,
				flagStage,
				getContext(),
				flagTimeout,
				isDryRun(),
				isVerbose(),
				isInsecure(),
			)
		},
	}

	cmd.Flags().StringVar(&flagPath, "path", "", "JSON values for {placeholder} substitution in api_path")
	cmd.Flags().StringVar(&flagQuery, "query", "", "JSON query parameters")
	cmd.Flags().StringVar(&flagBody, "body", "", "JSON request body")
	cmd.Flags().StringArrayVar(&flagHeaders, "header", nil, "Additional headers (key:value, repeatable)")
	cmd.Flags().StringVar(&flagStage, "stage", "prod", "API gateway stage (default: prod)")
	cmd.Flags().StringVar(&flagTimeout, "timeout", "", "Request timeout override (e.g. 180s)")

	return cmd
}

func runAPI(
	w io.Writer,
	gatewayName, method, apiPath, pathJSON, queryJSON, body string,
	headers []string,
	stage, ctxOverride, timeoutOverride string,
	dryRun, verbose bool,
	insecure bool,
) error {
	envelope, err := executeAPI(
		gatewayName,
		method,
		apiPath,
		pathJSON,
		queryJSON,
		body,
		headers,
		stage,
		ctxOverride,
		timeoutOverride,
		dryRun,
		verbose,
		insecure,
	)
	if err != nil {
		return err
	}

	return envelope.WriteJSON(w)
}

func executeAPI(
	gatewayName, method, apiPath, pathJSON, queryJSON, body string,
	headers []string,
	stage, ctxOverride, timeoutOverride string,
	dryRun, verbose, insecure bool,
) (*output.Envelope, error) {
	// Substitute path placeholders
	resolvedPath, err := api.SubstitutePath(apiPath, pathJSON)
	if err != nil {
		var fieldErr *validate.FieldError
		if errors.As(err, &fieldErr) && fieldErr.Field == "gateway_name" {
			return nil, output.UserError(
				"invalid_gateway_name",
				err.Error(),
				"Use a gateway name matching ^[a-z][a-z0-9-]{2,29}$",
			)
		}
		return nil, output.UserError("path_error", err.Error(), "Check --path JSON and api_path placeholders")
	}

	runtime, err := requestexec.ResolveRuntimeWithOptions(
		ctxOverride,
		dryRun,
		verbose,
		requestexec.RuntimeOptions{
			Insecure: insecure,
		},
	)
	if err != nil {
		return nil, err
	}

	result, err := requestexec.ExecuteRawAPIRequest(runtime, requestexec.RequestSpec{
		GatewayName: gatewayName,
		Method:      method,
		Path:        resolvedPath,
		ParamsJSON:  queryJSON,
		BodyJSON:    body,
		Headers:     headers,
		Stage:       stage,
		Timeout:     timeoutOverride,
	})
	if err != nil {
		return nil, err
	}

	return result.Envelope, nil
}
