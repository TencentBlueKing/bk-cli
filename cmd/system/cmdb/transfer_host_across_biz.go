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

func newTransferHostAcrossBizCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		srcBizID   int
		dstBizID   int
		hostIDsCSV string
		moduleID   int
		stage      string
		body       string
		headers    []string
	)

	cmd := &cobra.Command{
		Use:   "transfer_host_across_biz",
		Short: "Transfer hosts across businesses",
		Example: strings.Join([]string{
			"  bk-cli cmdb transfer_host_across_biz --src_bk_biz_id 1 --dst_bk_biz_id 2 --bk_host_ids 100,101 --bk_module_id 20",
			"  bk-cli cmdb transfer_host_across_biz --body '" +
				"{\"src_bk_biz_id\":1,\"dst_bk_biz_id\":2,\"bk_host_id\":[100],\"bk_module_id\":20}'",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			bodyJSON, err := buildTransferHostAcrossBizBody(body, srcBizID, dstBizID, hostIDsCSV, moduleID)
			if err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "transfer_host_across_biz", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        "/api/v3/open/hosts/modules/across/biz",
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

	cmd.Flags().IntVar(&srcBizID, "src_bk_biz_id", 0, "Source business ID")
	cmd.Flags().IntVar(&dstBizID, "dst_bk_biz_id", 0, "Destination business ID")
	cmd.Flags().StringVar(&hostIDsCSV, "bk_host_ids", "", "Comma-separated host IDs")
	cmd.Flags().IntVar(&moduleID, "bk_module_id", 0, "Destination module ID")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}

func buildTransferHostAcrossBizBody(
	bodyOverride string,
	srcBizID int,
	dstBizID int,
	hostIDsCSV string,
	moduleID int,
) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}
	if err := systemcmd.ValidatePositiveIntFlag("src_bk_biz_id", srcBizID); err != nil {
		return "", err
	}
	if err := systemcmd.ValidatePositiveIntFlag("dst_bk_biz_id", dstBizID); err != nil {
		return "", err
	}
	if err := systemcmd.ValidatePositiveIntFlag("bk_module_id", moduleID); err != nil {
		return "", err
	}
	hostIDs, err := utils.ParseCSVInts(hostIDsCSV, "bk_host_ids", "Use --bk_host_ids 1,2,3")
	if err != nil {
		return "", err
	}

	return systemcmd.MarshalJSON(map[string]any{
		"src_bk_biz_id": srcBizID,
		"dst_bk_biz_id": dstBizID,
		"bk_host_id":    hostIDs,
		"bk_module_id":  moduleID,
	})
}
