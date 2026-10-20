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

package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

func newLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store authentication credentials",
		Long: `Store authentication credentials for the active (or specified) context.

Initialize a context first with:
  bk-cli context init --bk_api_url_tmpl='https://bkapi.your-domain.com/api/{gateway_name}/'

Four credential modes are supported:
  1. App + token:   --bk_app_code + --bk_app_secret + --bk_token
  2. App + ticket:  --bk_app_code + --bk_app_secret + --bk_ticket
  3. Access token:  --access_token
  4. Personal token: --personal_token (exclusive; sent as Bearer authentication)

Environment fallbacks (read directly by the CLI):
  --bk_app_code   -> BK_CLI_BK_APP_CODE
  --bk_app_secret -> BK_CLI_BK_APP_SECRET
  --bk_token      -> BK_CLI_BK_TOKEN
  --bk_ticket     -> BK_CLI_BK_TICKET
  --access_token  -> BK_CLI_ACCESS_TOKEN
  --personal_token -> BK_CLI_PERSONAL_TOKEN

Explicit credential flags select the mode and take precedence; environment values
only fill missing fields in that mode. Explicit empty values are rejected.
--personal_token cannot be combined with any other credential flag, even an empty one.
Explicit flags for other modes ignore BK_CLI_PERSONAL_TOKEN; environment-only
personal login requires BK_CLI_PERSONAL_TOKEN without other credential variables.
Selected environment values with leading or trailing whitespace are rejected.
Without credential flags, the environment must contain only one credential mode.
With no credentials in either source, device flow remains unimplemented.
Inject credentials through your secret manager or CI environment, then run auth login
without credential flags to keep credential values out of the process argument list.

Use single quotes around literal credentials to prevent shell expansion of $ and command substitutions.
If a value contains a single quote, close the quotes, escape it, and reopen the quotes (e.g. 'it'\''s').

Tenant configuration belongs to the context, not the credential.
Use context init/create --tenant_id or a one-off request header override.

Examples:
  # After injecting one credential mode into the environment:
  bk-cli auth login
  bk-cli auth login --context dev
  bk-cli auth login --bk_app_code='app' --bk_app_secret='secret' --bk_token='tok'
  bk-cli auth login --access_token='my_token'
  bk-cli auth login --personal_token='<personal-token>'
  bk-cli context create dev --bk_api_url_tmpl='https://bkapi.example.com/api/{gateway_name}/' --tenant_id='t1'
  bk-cli context use dev
  bk-cli auth login --access_token='my_token'
  bk-cli auth login --context dev --access_token='my_token'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Personal mode is exclusive even when a legacy flag is explicitly empty.
			if cmd.Flags().Changed("personal_token") {
				for _, name := range []string{"bk_app_code", "bk_app_secret", "bk_token", "bk_ticket", "access_token"} {
					if cmd.Flags().Changed(name) {
						return output.UserError("invalid_credentials",
							"--personal_token cannot be combined with --"+name,
							"Use --personal_token alone. See: bk-cli auth login --help")
					}
				}
			}

			input, err := resolveLoginInput(cmd)
			if err != nil {
				return err
			}
			bkAppCode, bkAppSecret := input.BkAppCode, input.BkAppSecret
			bkToken, bkTicket, accessToken := input.BkToken, input.BkTicket, input.AccessToken
			personalToken := input.PersonalToken

			// No credential input → placeholder for device flow
			if bkAppCode == "" && bkAppSecret == "" && bkToken == "" && bkTicket == "" &&
				accessToken == "" && personalToken == "" {
				return output.UserError(
					"not_implemented",
					"OAuth2 device flow not yet implemented",
					"Provide credentials via flags or environment variables. See: bk-cli auth login --help",
				)
			}

			// Resolve context
			ctxOverride, _ := cmd.Root().PersistentFlags().GetString("context")
			ctxName, cfg, err := config.ResolveContext(ctxOverride)
			if err != nil {
				return output.UserError(
					"context_error",
					err.Error(),
					"Run: bk-cli context init --bk_api_url_tmpl=URL",
				)
			}

			// Build credential
			var cred *credential.Credential
			switch {
			case personalToken != "":
				cred = &credential.Credential{
					Type:          credential.TypePersonalToken,
					PersonalToken: personalToken,
				}
			case accessToken != "":
				cred = &credential.Credential{
					Type:        credential.TypeAccessToken,
					AccessToken: accessToken,
				}
			default:
				// Determine which user key to set based on config.UserKey
				userKey := cfg.UserKey
				if userKey == "" {
					userKey = config.DefaultUserKey
				}

				if bkAppCode != "" && bkAppSecret != "" && bkToken == "" && bkTicket == "" {
					return output.UserError(
						"invalid_credentials",
						fmt.Sprintf(
							"app credential mode requires a user credential; provide --bk_token or --bk_ticket "+
								"or set BK_CLI_BK_TOKEN or BK_CLI_BK_TICKET "+
								"together with --bk_app_code and --bk_app_secret (current context defaults to --%s)",
							userKey,
						),
						fmt.Sprintf(
							"Example: bk-cli auth login --bk_app_code='APP_CODE' --bk_app_secret='APP_SECRET' "+
								"--%s='VALUE'; or set BK_CLI_BK_TOKEN or BK_CLI_BK_TICKET and run bk-cli auth login; "+
								"or use --access_token",
							userKey,
						),
					)
				}

				cred = &credential.Credential{
					Type:        credential.TypeAppUser,
					BkAppCode:   bkAppCode,
					BkAppSecret: bkAppSecret,
				}

				// If user explicitly provides bk_ticket, use it; if bk_token, use it.
				// Otherwise fall back to config.UserKey to decide which field.
				switch {
				case bkTicket != "":
					cred.BkTicket = bkTicket
				case bkToken != "":
					cred.BkToken = bkToken
				case userKey == "bk_ticket":
					cred.BkTicket = bkTicket
				default:
					cred.BkToken = bkToken
				}
			}

			// Validate
			if err := cred.Validate(); err != nil {
				return output.UserError(
					"invalid_credentials",
					err.Error(),
					"Provide a complete credential set. See: bk-cli auth login --help",
				)
			}

			// Derive encryption key and save
			key, err := credential.DeriveKey()
			if err != nil {
				return output.SystemError("encryption_error", err.Error(), "")
			}

			credPath := config.CredentialsPath(ctxName)
			if err := credential.Save(credPath, cred, key); err != nil {
				return output.SystemError("save_error", err.Error(), "")
			}

			return output.Success(
				fmt.Sprintf("Credentials saved for context %q", ctxName),
			).WriteJSON(
				cmd.OutOrStdout(),
			)
		},
	}

	cmd.Flags().String("bk_app_code", "", "BlueKing app code (env: BK_CLI_BK_APP_CODE)")
	cmd.Flags().String("bk_app_secret", "", "BlueKing app secret (env: BK_CLI_BK_APP_SECRET)")
	cmd.Flags().String("bk_token", "", "User token (env: BK_CLI_BK_TOKEN)")
	cmd.Flags().String("bk_ticket", "", "User ticket (env: BK_CLI_BK_TICKET)")
	cmd.Flags().String(
		"access_token",
		"",
		"Access token (standalone mode; env: BK_CLI_ACCESS_TOKEN)",
	)

	cmd.Flags().String("personal_token", "", "Personal token (exclusive Bearer mode; env: BK_CLI_PERSONAL_TOKEN)")

	return cmd
}
