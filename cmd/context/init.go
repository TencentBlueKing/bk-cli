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
	"time"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

// newInitCmd initializes the first/default context explicitly.
func newInitCmd() *cobra.Command {
	var (
		bkAPIURLTmpl string
		bkAuthURL    string
		tenantID     string
		userKey      string
		timeout      time.Duration
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the default context",
		Long: `Initialize the default context for the current BlueKing deployment.

This is the required first-run setup. It creates the "default" context,
saves the provided URL template, and marks it as active.

Examples:
  bk-cli context init --bk_api_url_tmpl="https://bkapi.example.com/api/{gateway_name}/"
  bk-cli context init --bk_api_url_tmpl="https://bkapi.example.com/api/{gateway_name}/" --tenant_id=mytenant`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := runCreateContext(
				"default",
				bkAPIURLTmpl,
				bkAuthURL,
				tenantID,
				userKey,
				timeout,
				true,
			); err != nil {
				return err
			}

			return output.Success(
				"Context \"default\" created and set as active",
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
