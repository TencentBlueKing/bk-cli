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

package pipeline

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newStartBuildCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		projectID  string
		pipelineID string
		buildNo    int
		stage      string
		body       string
		headers    []string
	)

	cmd := &cobra.Command{
		Use:   "start_build",
		Short: "Start a build",
		Long: `Start a new pipeline build.

When --body is not provided, an empty JSON object is sent as the build parameters.
Use --body to pass custom startup parameters as a JSON key-value object.`,
		Example: strings.Join([]string{
			"  bk-cli devops pipeline start_build --projectId myproject --pipelineId p-xxx",
			`  bk-cli devops pipeline start_build --projectId myproject --pipelineId p-xxx --body '{"param1":"value1"}'`,
			"  bk-cli devops pipeline start_build --projectId myproject --pipelineId p-xxx --buildNo 5",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			if err := systemcmd.ValidateNonEmptyStringFlag("projectId", projectID); err != nil {
				return err
			}
			if err := systemcmd.ValidateNonEmptyStringFlag("pipelineId", pipelineID); err != nil {
				return err
			}
			if cmd.Flags().Changed("buildNo") {
				if err := systemcmd.ValidatePositiveIntFlagIfChanged(
					"buildNo",
					buildNo,
					true,
				); err != nil {
					return err
				}
			}

			bodyJSON := body
			if bodyJSON == "" {
				bodyJSON = "{}"
			}

			query := url.Values{}
			query.Set("pipelineId", pipelineID)
			if cmd.Flags().Changed("buildNo") {
				query.Set("buildNo", strconv.Itoa(buildNo))
			}

			path := fmt.Sprintf(
				"/v4/apigw-user/projects/%s/build_start?%s",
				url.PathEscape(projectID),
				query.Encode(),
			)

			return systemcmd.ExecuteRequest(cmd, runtime, "start_build", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        path,
				BodyJSON:    bodyJSON,
				Headers:     headers,
				Stage:       stage,
				AuthConfig: &syslib.AuthConfig{
					AppVerifiedRequired:        false,
					UserVerifiedRequired:       true,
					ResourcePermissionRequired: false,
				},
			}, nil)
		},
	}

	cmd.Flags().StringVar(&projectID, "projectId", "", "Project ID (English project name)")
	cmd.Flags().StringVar(&pipelineID, "pipelineId", "", "Pipeline ID (string starting with p-)")
	cmd.Flags().IntVar(&buildNo, "buildNo", 0, "Build number to start")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}
