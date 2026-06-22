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
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/credential"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show credential status for the active context",
		Long: `Display stored credential information for the active (or specified) context.

This command always returns a success envelope. If the target context has no
stored credentials, the result is reported as has_credentials=false.

Examples:
  bk-cli auth status
  bk-cli auth status --context=clouds`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxName, cred, hasCredentials, err := resolveStoredCredential(cmd)
			if err != nil {
				return err
			}

			data := map[string]any{
				"context":         ctxName,
				"has_credentials": hasCredentials,
			}

			if hasCredentials {
				data["credential_type"] = string(cred.Type)
				if cred.Type == credential.TypeAppUser {
					data["bk_app_code"] = cred.MaskedAppCode()
					data["user_key"] = cred.UserKeyType()
				}
			}

			return output.SuccessData(data).WriteJSON(cmd.OutOrStdout())
		},
	}
}
