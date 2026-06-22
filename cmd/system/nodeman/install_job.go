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

package nodeman

import (
	"github.com/spf13/cobra"

	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newInstallJobCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		stage   string
		body    string
		headers []string
	)

	cmd := &cobra.Command{
		Use:   "install_job",
		Short: "Create an install/reinstall/uninstall/upgrade job for Agent or Proxy",
		Long: `Create an install-type job on the Node Management platform.

The hosts payload is complex; use --body to pass the full JSON request body directly.
Supported job_type values: INSTALL_AGENT, INSTALL_PROXY, REINSTALL_AGENT, REINSTALL_PROXY,
REPLACE_PROXY, UNINSTALL_AGENT, UNINSTALL_PROXY, UPGRADE_AGENT, UPGRADE_PROXY,
RELOAD_AGENT, RELOAD_PROXY.`,
		Example: "  bk-cli nodeman install_job \\\n" +
			"    --body '<json>'",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			if err := systemcmd.ValidateNonEmptyStringFlag("body", body); err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "install_job", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        "/api/job/install/",
				BodyJSON:    body,
				Headers:     headers,
				Stage:       stage,
				AuthConfig: &syslib.AuthConfig{
					AppVerifiedRequired:        true,
					UserVerifiedRequired:       false,
					ResourcePermissionRequired: true,
				},
			}, nil)
		},
	}

	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}
