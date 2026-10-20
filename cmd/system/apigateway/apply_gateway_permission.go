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

package apigateway

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/api"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

func newApplyGatewayPermissionCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		gatewayName string
		confirmed   bool
		stage, body string
		headers     []string
	)
	cmd := &cobra.Command{
		Use:   "apply_gateway_permission",
		Short: "Apply for gateway or batch API permissions after user confirmation",
		Long: `Apply for permissions through v2_open_apply_gateway_permission.

Agents must ask the user for explicit confirmation before every submission.
Explain the target gateway, application, resources, expiry and reason: submission
creates an application record and triggers notifications and an approval workflow.
This confirmation cannot be skipped. Set --confirmed only after the user agrees
to this exact request; it records their consent and does not replace asking them.
Batch required resource_names into one request per gateway to avoid excessive
application records. Do not retry automatically: a retry can create another record.
Use --dry-run to preview without confirmation or submitting an application.

This API requires application identity and does not support personal_token.
Personal-token contexts and explicit Authorization: Bearer headers are rejected
locally with unsupported_credential_type, including during --dry-run. Select an
application credential context or log in with app credentials or a suitable
access_token before previewing or submitting.

The required --body accepts JSON, @file or - for stdin.
Before preview or submission, the shared request layer reads the input and checks
JSON syntax. Invalid JSON returns request_error. The original body is sent unchanged;
the server validates its structure, fields and whether an application is allowed.

Body fields:
  target_app_code: required; application requesting permission, matching the caller
  grant_dimension: required; gateway (all APIs) or resource (selected APIs)
  resource_names: required for resource; array of API names to apply for together
  applicant: required; applicant username
  expire_days: optional; 0 or omitted = permanent, 180 = 6 months, 360 = 12 months
  reason: optional; explain why access is needed

The JSON output preserves the complete upstream response, including record_id
and any approval links or approvers the server returns. Agents must present these
returned details to the user. Submission does not mean permission is granted.
record_id is a positive integer, preserved without rounding even for large IDs.
The API may also return itsm_ticket_id and itsm_ticket_url (possibly empty).
Do not invent approval links or approvers, or assume the CLI knows approval status.`,
		Example: `  # Preview a batch, then ask the user to confirm this exact request
  bk-cli apigateway apply_gateway_permission --gateway_name bk-iam --body @permission.json --dry-run
  # Only after explicit user confirmation
  bk-cli apigateway apply_gateway_permission --gateway_name bk-iam --confirmed \
    --body '{"target_app_code":"my-app","grant_dimension":"resource",
      "resource_names":["user_query","user_create"],"applicant":"developer",
      "expire_days":180,"reason":"Read and create users"}'
  # Apply for all APIs in a gateway only if the user explicitly confirmed that scope
  bk-cli apigateway apply_gateway_permission --gateway_name bk-iam --confirmed \
    --body '{"target_app_code":"my-app","grant_dimension":"gateway",
      "applicant":"developer","reason":"Gateway integration"}'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dryRun := deps.IsDryRun != nil && deps.IsDryRun()
			if !dryRun && !confirmed {
				return output.UserError(
					"confirmation_required",
					"explicit user confirmation is required before applying for permissions",
					"Ask the user to confirm the gateway, application, resources, expiry and reason; "+
						"then pass --confirmed. Use --dry-run to preview without submitting.",
				)
			}
			if err := systemcmd.ValidateNonEmptyStringFlag("gateway_name", gatewayName); err != nil {
				return err
			}
			if err := validate.ValidateGatewayName(gatewayName); err != nil {
				return output.UserError("invalid_gateway_name", err.Error(),
					"Use a gateway name matching ^[a-z][a-z0-9-]{2,29}$")
			}
			if err := systemcmd.ValidateNonEmptyStringFlag("body", body); err != nil {
				return err
			}
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}
			headerMap, err := api.ParseHeaderFlags(headers)
			if err != nil {
				return output.UserError(
					"request_error",
					err.Error(),
					"Use repeatable --header key:value entries",
				)
			}
			if err := runtime.EnsureCredential(); err != nil {
				return err
			}
			if err := validatePersonalTokenAuth(runtime.Credential, headerMap); err != nil {
				return err
			}
			return systemcmd.ExecuteRequest(cmd, runtime, "apply_gateway_permission", syslib.RequestSpec{
				GatewayName: "bk-apigateway",
				Method:      "POST",
				Path:        "/api/v2/open/gateways/" + gatewayName + "/permissions/apply/",
				BodyJSON:    body,
				Headers:     headers,
				Stage:       stage,
				AuthConfig: &syslib.AuthConfig{
					AppVerifiedRequired:        true,
					UserVerifiedRequired:       false,
					ResourcePermissionRequired: false,
				},
			}, func(env *output.Envelope) error {
				if env.DryRun || !env.OK {
					return nil
				}
				upstream, ok := env.Data.(map[string]any)
				if !ok {
					return nil
				}
				data, ok := upstream["data"].(map[string]any)
				if !ok {
					return nil
				}
				if recordID, ok := data["record_id"]; ok {
					env.Message = fmt.Sprintf(
						"Permission application submitted (record_id: %v); ",
						recordID,
					) +
						"this does not mean permission has been granted. " +
						"Present any returned approval links or approvers to the user."
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&gatewayName, "gateway_name", "", "[Required] Gateway whose permissions are requested")
	cmd.Flags().BoolVar(
		&confirmed,
		"confirmed",
		false,
		"[Required for submission] Attest that the user explicitly confirmed this exact request; never skip asking",
	)
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)
	return cmd
}

func validatePersonalTokenAuth(cred *credential.Credential, headers map[string]string) error {
	personalAuth := cred != nil && cred.Type == credential.TypePersonalToken
	for name, value := range headers {
		if strings.EqualFold(name, "Authorization") {
			parts := strings.Fields(value)
			personalAuth = personalAuth || (len(parts) > 0 && strings.EqualFold(parts[0], "Bearer"))
		}
	}
	if personalAuth {
		return output.UserError(
			"unsupported_credential_type",
			"personal_token authentication is not supported for this action; application identity is required",
			"Select an application credential context using --context, or run bk-cli auth login with "+
				"--bk_app_code/--bk_app_secret and --bk_token/--bk_ticket, or a suitable --access_token; "+
				"remove any Authorization: Bearer override before previewing or submitting.",
		)
	}
	return nil
}
