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

package sops

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newStartTaskCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		bizID   int
		taskID  int
		stage   string
		body    string
		headers []string
	)

	cmd := &cobra.Command{
		Use:   "start_task",
		Short: "Start executing a created task",
		Example: strings.Join([]string{
			"  bk-cli sops start_task --bk_biz_id 2 --task_id 100",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			if err := systemcmd.ValidatePositiveIntFlag("bk_biz_id", bizID); err != nil {
				return err
			}
			if err := systemcmd.ValidatePositiveIntFlag("task_id", taskID); err != nil {
				return err
			}

			bodyJSON := body
			if bodyJSON == "" {
				bodyJSON = "{}"
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "start_task", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        fmt.Sprintf("/start_task/%d/%d/", taskID, bizID),
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

	cmd.Flags().IntVar(&bizID, "bk_biz_id", 0, "Business ID (required)")
	cmd.Flags().IntVar(&taskID, "task_id", 0, "Task ID (required)")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}
