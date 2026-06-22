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

package stream

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newTriggerCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		projectID  string
		pipelineID string
		stage      string
		body       string
		headers    []string
	)

	cmd := &cobra.Command{
		Use:   "trigger",
		Short: "Trigger a Stream pipeline",
		Long: `Trigger a Stream pipeline.

This command requires an explicit JSON request body that matches the upstream trigger contract.`,
		Example: strings.Join([]string{
			"  bk-cli devops stream trigger \\",
			"    --projectId git_12345 \\",
			"    --pipelineId p-xxx \\",
			`    --body '{"path":".ci/demo.yml","branch":"main","projectId":"git_12345","customCommitMsg":"manual trigger"}'`,
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
			if err := systemcmd.ValidateNonEmptyStringFlag("body", body); err != nil {
				return err
			}

			query := url.Values{}
			query.Set("pipelineId", pipelineID)

			path := fmt.Sprintf(
				"/v4/apigw-user/stream/gitProjects/%s/openapi_trigger?%s",
				url.PathEscape(projectID),
				query.Encode(),
			)

			return systemcmd.ExecuteRequest(cmd, runtime, "trigger", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        path,
				BodyJSON:    body,
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

	cmd.Flags().StringVar(
		&projectID,
		"projectId",
		"",
		"BlueKing project ID (fixed format: git_${GongfengProjectId})",
	)
	cmd.Flags().StringVar(&pipelineID, "pipelineId", "", "Pipeline ID (p-xxx)")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}
