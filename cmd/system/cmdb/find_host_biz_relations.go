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

package cmdb

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
	"github.com/TencentBlueKing/bk-cli/internal/utils"
)

func newFindHostBizRelationsCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		hostIDsCSV string
		stage      string
		body       string
		headers    []string
	)

	cmd := &cobra.Command{
		Use:   "find_host_biz_relations",
		Short: "Find host business and module relations",
		Example: strings.Join([]string{
			"  bk-cli cmdb find_host_biz_relations --bk_host_ids 1,2,3",
			"  bk-cli cmdb find_host_biz_relations --body '{\"bk_host_id\":[1,2,3]}'",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			bodyJSON, err := buildFindHostBizRelationsBody(body, hostIDsCSV)
			if err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "find_host_biz_relations", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        "/api/v3/open/hosts/modules/read",
				BodyJSON:    bodyJSON,
				Headers:     headers,
				Stage:       stage,
				AuthConfig: &syslib.AuthConfig{
					AppVerifiedRequired:        true,
					UserVerifiedRequired:       true,
					ResourcePermissionRequired: true,
				},
			}, func(env *output.Envelope) error {
				if env.DryRun {
					return nil
				}
				if _, ok := env.Data.([]any); !ok {
					env.Data = []any{}
				}
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&hostIDsCSV, "bk_host_ids", "", "Comma-separated host IDs")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}

func buildFindHostBizRelationsBody(bodyOverride, hostIDsCSV string) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}
	hostIDs, err := utils.ParseCSVInts(hostIDsCSV, "bk_host_ids", "Use --bk_host_ids 1,2,3")
	if err != nil {
		return "", err
	}
	return systemcmd.MarshalJSON(map[string]any{"bk_host_id": hostIDs})
}
