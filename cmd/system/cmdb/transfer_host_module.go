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

func newTransferHostModuleCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		bizID        int
		hostIDsCSV   string
		moduleIDsCSV string
		isIncrement  bool
		stage        string
		body         string
		headers      []string
	)

	cmd := &cobra.Command{
		Use:   "transfer_host_module",
		Short: "Transfer hosts to modules inside one business",
		Example: strings.Join([]string{
			"  bk-cli cmdb transfer_host_module --bk_biz_id 2 --bk_host_ids 1,2 --bk_module_ids 20,30",
			"  bk-cli cmdb transfer_host_module --bk_biz_id 2 --bk_host_ids 1,2 --bk_module_ids 20 --is_increment",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			bodyJSON, err := buildTransferHostModuleBody(body, bizID, hostIDsCSV, moduleIDsCSV, isIncrement)
			if err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "transfer_host_module", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        "/api/v3/open/hosts/modules",
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

	cmd.Flags().IntVar(&bizID, "bk_biz_id", 0, "Business ID")
	cmd.Flags().StringVar(&hostIDsCSV, "bk_host_ids", "", "Comma-separated host IDs")
	cmd.Flags().StringVar(&moduleIDsCSV, "bk_module_ids", "", "Comma-separated module IDs")
	cmd.Flags().BoolVar(&isIncrement, "is_increment", false, "Incrementally add modules instead of replacing them")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}

func buildTransferHostModuleBody(
	bodyOverride string,
	bizID int,
	hostIDsCSV string,
	moduleIDsCSV string,
	isIncrement bool,
) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}
	if err := systemcmd.ValidatePositiveIntFlag("bk_biz_id", bizID); err != nil {
		return "", err
	}
	hostIDs, err := utils.ParseCSVInts(hostIDsCSV, "bk_host_ids", "Use --bk_host_ids 1,2,3")
	if err != nil {
		return "", err
	}
	moduleIDs, err := utils.ParseCSVInts(moduleIDsCSV, "bk_module_ids", "Use --bk_module_ids 20,30")
	if err != nil {
		return "", err
	}

	return systemcmd.MarshalJSON(map[string]any{
		"bk_biz_id":    bizID,
		"bk_host_id":   hostIDs,
		"bk_module_id": moduleIDs,
		"is_increment": isIncrement,
	})
}
