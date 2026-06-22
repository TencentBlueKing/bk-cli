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

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials for the active context",
		Long: `Remove stored credentials for the active (or specified) context.

Examples:
  bk-cli auth logout
  bk-cli auth logout --context=clouds`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxOverride, _ := cmd.Root().PersistentFlags().GetString("context")
			ctxName, _, err := config.ResolveContext(ctxOverride)
			if err != nil {
				return output.UserError(
					"context_error",
					err.Error(),
					"Run: bk-cli context init --bk_api_url_tmpl=URL",
				)
			}

			credPath := config.CredentialsPath(ctxName)
			if err := credential.Delete(credPath); err != nil {
				return output.SystemError("delete_error", err.Error(), "")
			}

			return output.Success(
				fmt.Sprintf("Credentials removed for context %q", ctxName),
			).WriteJSON(
				cmd.OutOrStdout(),
			)
		},
	}
}
