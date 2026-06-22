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

package context

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

func runCreateContext(
	name,
	bkAPIURLTmpl,
	bkAuthURL,
	tenantID,
	userKey string,
	timeout time.Duration,
	setActive bool,
) error {
	if err := validate.ValidateContextName(name); err != nil {
		return output.UserError(
			"invalid_context_name",
			err.Error(),
			"Use a safe context name like default, prod-1, or clouds",
		)
	}
	if err := config.ValidateURLTemplate(bkAPIURLTmpl); err != nil {
		return output.UserError(
			"invalid_url_template",
			err.Error(),
			"URL template must contain {gateway_name} placeholder, e.g. https://bkapi.example.com/api/{gateway_name}/",
		)
	}

	cfg := &config.Config{
		BkAPIURLTmpl: bkAPIURLTmpl,
		BkAuthURL:    bkAuthURL,
		TenantID:     tenantID,
		UserKey:      userKey,
		Timeout:      timeout,
	}

	if err := config.CreateContext(name, cfg); err != nil {
		return output.UserError("create_context_failed", err.Error(), "")
	}

	if setActive {
		if err := config.SetActiveContext(name); err != nil {
			return output.UserError("set_active_failed", err.Error(), "")
		}
	}

	return nil
}

func newCreateCmd() *cobra.Command {
	var (
		bkAPIURLTmpl string
		bkAuthURL    string
		tenantID     string
		userKey      string
		timeout      time.Duration
	)

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new context",
		Long: `Create a new named context targeting a BlueKing deployment.

The new context is created without changing the active context.
Use "bk-cli context use <name>" when you want to switch.

Examples:
  bk-cli context create clouds --bk_api_url_tmpl="https://bkapi.clouds.example.com/api/{gateway_name}/"
  bk-cli context create dev --bk_api_url_tmpl="https://bkapi.dev.example.com/api/{gateway_name}/" --tenant_id=mytenant`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if err := runCreateContext(
				name,
				bkAPIURLTmpl,
				bkAuthURL,
				tenantID,
				userKey,
				timeout,
				false,
			); err != nil {
				return err
			}

			return output.Success(
				fmt.Sprintf("Context %q created", name),
			).WriteJSON(
				cmd.OutOrStdout(),
			)
		},
	}

	cmd.Flags().StringVar(
		&bkAPIURLTmpl,
		"bk_api_url_tmpl",
		"",
		"BlueKing API URL template (required, must contain {gateway_name})",
	)
	_ = cmd.MarkFlagRequired("bk_api_url_tmpl")
	cmd.Flags().StringVar(&bkAuthURL, "bk_auth_url", "", "BlueKing auth URL (optional)")
	cmd.Flags().StringVar(&tenantID, "tenant_id", "", "Tenant ID (optional)")
	cmd.Flags().StringVar(&userKey, "user_key", "bk_token", "User key field name")
	cmd.Flags().DurationVar(
		&timeout,
		"timeout",
		config.DefaultTimeout,
		"Default request timeout for this context, e.g. 30s, 1m, 2m30s",
	)

	return cmd
}
