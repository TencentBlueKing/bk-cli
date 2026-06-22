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

package job

import (
	"github.com/spf13/cobra"

	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newFastTransferFileCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		stage   string
		body    string
		headers []string
	)

	cmd := &cobra.Command{
		Use:   "fast_transfer_file",
		Short: "Fast distribute files to target hosts",
		Long: `Distribute files from source to target hosts.

The file source/target payload is complex; use --body to pass the full JSON request body.
Required body fields: bk_biz_id, file_source_list, file_target_path, target_server.
Optional: account_alias, account_id, timeout, task_name, download_speed_limit,
upload_speed_limit, transfer_mode (1=strict, 2=force).`,
		Example: "  bk-cli job fast_transfer_file \\\n" +
			"    --body '<json>'",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			if err := systemcmd.ValidateNonEmptyStringFlag("body", body); err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "fast_transfer_file", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        "/api/v3/fast_transfer_file",
				BodyJSON:    body,
				Headers:     headers,
				Stage:       stage,
				AuthConfig: &syslib.AuthConfig{
					AppVerifiedRequired:        true,
					UserVerifiedRequired:       true,
					ResourcePermissionRequired: true,
				},
			}, nil)
		},
	}

	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}
