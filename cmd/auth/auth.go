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

// Package auth defines authentication-related Cobra commands.
package auth

import "github.com/spf13/cobra"

// NewAuthCmd creates the parent auth command.
func NewAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication credentials",
		Long: `Manage authentication credentials for BlueKing platform.

Examples:
  # First-run setup
  bk-cli context init --bk_api_url_tmpl="https://bkapi.example.com/api/{gateway_name}/"

  # Login with app credentials and user token
  bk-cli auth login --bk_app_code="app" --bk_app_secret="secret" --bk_token="tok"

  # Login with access token
  bk-cli auth login --access_token="my_token"

  # Check credential status
  bk-cli auth status

  # Fail fast when credentials are required
  bk-cli auth check

  # Remove stored credentials
  bk-cli auth logout`,
	}
	cmd.AddCommand(newLoginCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newCheckCmd())
	cmd.AddCommand(newLogoutCmd())
	return cmd
}
