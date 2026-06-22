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
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
	"github.com/TencentBlueKing/bk-cli/internal/utils"
)

func newDeleteHostCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		hostIDsCSV string
		stage      string
		body       string
		headers    []string
	)

	cmd := &cobra.Command{
		Use:   "delete_host",
		Short: "Delete hosts from the resource pool",
		Example: strings.Join([]string{
			"  bk-cli cmdb delete_host --bk_host_ids 100,200",
			"  bk-cli cmdb delete_host --body '{\"bk_host_id\":\"100,200\"}'",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			bodyJSON, err := buildDeleteHostBody(body, hostIDsCSV)
			if err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "delete_host", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "DELETE",
				Path:        "/api/v3/open/hosts/batch",
				BodyJSON:    bodyJSON,
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

	cmd.Flags().StringVar(&hostIDsCSV, "bk_host_ids", "", "Comma-separated host IDs")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}

func buildDeleteHostBody(bodyOverride, hostIDsCSV string) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}
	hostIDs, err := utils.ParseCSVInts(hostIDsCSV, "bk_host_ids", "Use --bk_host_ids 100,200")
	if err != nil {
		return "", err
	}
	// CMDB DELETE /api/v3/open/hosts/batch expects bk_host_id as a
	// comma-separated string (e.g. "100,200"), not a JSON int array.
	values := make([]string, 0, len(hostIDs))
	for _, hostID := range hostIDs {
		values = append(values, strconv.Itoa(hostID))
	}
	return systemcmd.MarshalJSON(map[string]any{
		"bk_host_id": strings.Join(values, ","),
	})
}
