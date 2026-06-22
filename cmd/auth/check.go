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

	"github.com/TencentBlueKing/bk-cli/internal/output"
)

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Fail fast when the active context has no stored credentials",
		Long: `Check whether the active (or specified) context has usable stored credentials.

This command is intended for scripts, CI, and guard checks.
It exits non-zero when credentials are missing.

Examples:
  bk-cli auth check
  bk-cli auth check --context=clouds`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxName, cred, hasCredentials, err := resolveStoredCredential(cmd)
			if err != nil {
				return err
			}
			if !hasCredentials {
				return output.UserError(
					"no_credentials",
					fmt.Sprintf("No credentials found for context %q", ctxName),
					"Run: bk-cli auth login --help",
				)
			}

			return output.SuccessData(map[string]any{
				"context":         ctxName,
				"credential_type": string(cred.Type),
				"has_credentials": true,
			}).WriteJSON(cmd.OutOrStdout())
		},
	}
}
