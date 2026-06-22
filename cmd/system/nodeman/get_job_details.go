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
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newGetJobDetailsCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		jobID    int
		page     int
		pagesize int
		stage    string
		body     string
		headers  []string
	)

	cmd := &cobra.Command{
		Use:   "get_job_details",
		Short: "Query job execution details",
		Example: strings.Join([]string{
			"  bk-cli nodeman get_job_details --job_id 123",
			"  bk-cli nodeman get_job_details --job_id 123 --page 1 --pagesize 50",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			if err := systemcmd.ValidatePositiveIntFlag("job_id", jobID); err != nil {
				return err
			}

			bodyJSON, err := buildGetJobDetailsBody(body, page, pagesize)
			if err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "get_job_details", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        fmt.Sprintf("/api/job/%d/details/", jobID),
				BodyJSON:    bodyJSON,
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

	cmd.Flags().IntVar(&jobID, "job_id", 0, "Job ID (required)")
	cmd.Flags().IntVar(&page, "page", 1, "Page number")
	cmd.Flags().IntVar(&pagesize, "pagesize", -1, "Page size (-1 for all)")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}

func buildGetJobDetailsBody(bodyOverride string, page, pagesize int) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}
	return systemcmd.MarshalJSON(map[string]any{
		"page":     page,
		"pagesize": pagesize,
	})
}
