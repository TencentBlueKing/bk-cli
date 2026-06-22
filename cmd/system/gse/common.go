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

package gse

import (
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
	"github.com/TencentBlueKing/bk-cli/internal/utils"
)

const (
	gatewayName        = "bk-gse"
	maxAgentIDListSize = 1000
)

type agentListCommandSpec struct {
	Name    string
	Short   string
	Example string
	Path    string
}

func buildAgentListBody(bodyOverride, agentIDsCSV string) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}
	ids := utils.ParseCSVFields(agentIDsCSV)
	if len(ids) == 0 {
		return "", systemcmd.ValidateNonEmptyStringFlag("agent_id_list", "")
	}
	if len(ids) > maxAgentIDListSize {
		return "", output.UserError(
			"invalid_argument",
			"agent_id_list cannot contain more than 1000 entries",
			"Use --agent_id_list with at most 1000 comma-separated agent IDs",
		)
	}

	return systemcmd.MarshalJSON(map[string]any{
		"agent_id_list": ids,
	})
}

func newAgentListCommand(spec agentListCommandSpec, deps systemcmd.BuildDeps) *cobra.Command {
	var (
		agentIDsCSV string
		stage       string
		body        string
		headers     []string
	)

	cmd := &cobra.Command{
		Use:     spec.Name,
		Short:   spec.Short,
		Example: spec.Example,
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			bodyJSON, err := buildAgentListBody(body, agentIDsCSV)
			if err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, spec.Name, syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        spec.Path,
				BodyJSON:    bodyJSON,
				Headers:     headers,
				Stage:       stage,
				AuthConfig: &syslib.AuthConfig{
					AppVerifiedRequired:        true,
					UserVerifiedRequired:       false,
					ResourcePermissionRequired: false,
				},
			}, nil)
		},
	}

	cmd.Flags().StringVar(&agentIDsCSV, "agent_id_list", "",
		"Comma-separated CMDB bk_agent_id values (max 1000; not 0:IP format)")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}
