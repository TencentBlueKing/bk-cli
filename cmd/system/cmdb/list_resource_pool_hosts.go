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

	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
	"github.com/TencentBlueKing/bk-cli/internal/utils"
)

func newListResourcePoolHostsCmd(deps systemcmd.BuildDeps) *cobra.Command {
	const defaultLimit = 500

	var (
		hostIPsRaw string
		fieldsCSV  string
		start      int
		limit      int
		stage      string
		body       string
		headers    []string
	)

	cmd := &cobra.Command{
		Use:   "list_resource_pool_hosts",
		Short: "List hosts in the CMDB resource pool",
		Example: strings.Join([]string{
			"  bk-cli cmdb list_resource_pool_hosts",
			"  bk-cli cmdb list_resource_pool_hosts --host_ips 10.0.0.1 " +
				"--fields bk_host_id,bk_host_innerip --start 10 --limit 100",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			bodyJSON, err := buildListResourcePoolHostsBody(body, fieldsCSV, hostIPsRaw, start, limit)
			if err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "list_resource_pool_hosts", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        "/api/v3/open/hosts/list_resource_pool_hosts",
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

	cmd.Flags().StringVar(
		&hostIPsRaw,
		"host_ips",
		"",
		"Comma-separated host IPs; use cloud:ip for non-zero cloud IDs",
	)
	cmd.Flags().StringVar(&fieldsCSV, "fields", "", "Comma-separated host fields to return")
	cmd.Flags().IntVar(&start, "start", 0, "Pagination start offset")
	cmd.Flags().IntVar(&limit, "limit", defaultLimit, "Maximum number of hosts to return")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}

func buildListResourcePoolHostsBody(
	bodyOverride, fieldsCSV, hostIPsRaw string,
	start, limit int,
) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}
	if err := systemcmd.ValidateNonNegativeIntFlag("start", start); err != nil {
		return "", err
	}
	if err := systemcmd.ValidatePositiveIntFlag("limit", limit); err != nil {
		return "", err
	}

	payload := map[string]any{
		"page": map[string]any{
			"start": start,
			"limit": limit,
			"sort":  "bk_host_id",
		},
	}
	if fields := utils.ParseCSVFields(fieldsCSV); len(fields) > 0 {
		payload["fields"] = fields
	}
	if strings.TrimSpace(hostIPsRaw) != "" {
		hostIPs, err := parseHostIPs(hostIPsRaw)
		if err != nil {
			return "", err
		}
		payload["host_property_filter"] = buildHostPropertyFilter(hostIPs)
	}

	return systemcmd.MarshalJSON(payload)
}
