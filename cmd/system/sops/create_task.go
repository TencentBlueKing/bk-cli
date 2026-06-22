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

func newCreateTaskCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		bizID      int
		templateID int
		name       string
		constants  string
		stage      string
		body       string
		headers    []string
	)

	cmd := &cobra.Command{
		Use:   "create_task",
		Short: "Create a task from a flow template",
		Example: strings.Join([]string{
			"  bk-cli sops create_task --bk_biz_id 2 --template_id 100 --name \"deploy-v1\"",
			`  bk-cli sops create_task --bk_biz_id 2 --template_id 100 --name "deploy" --constants '{"${key}":"value"}'`,
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			bodyJSON, err := buildCreateTaskBody(body, name, constants)
			if err != nil {
				return err
			}

			if err := systemcmd.ValidatePositiveIntFlag("bk_biz_id", bizID); err != nil {
				return err
			}
			if err := systemcmd.ValidatePositiveIntFlag("template_id", templateID); err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "create_task", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        fmt.Sprintf("/create_task/%d/%d/", templateID, bizID),
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
	cmd.Flags().IntVar(&templateID, "template_id", 0, "Template ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "Task name (required)")
	cmd.Flags().StringVar(&constants, "constants", "", "Template constants as JSON object")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}

func buildCreateTaskBody(bodyOverride, name, constants string) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}
	if err := systemcmd.ValidateNonEmptyStringFlag("name", name); err != nil {
		return "", err
	}
	payload := map[string]any{
		"name":      name,
		"flow_type": "common",
	}
	if constants != "" {
		decodedConstants, err := systemcmd.ParseJSONObjectFlag("constants", constants)
		if err != nil {
			return "", err
		}
		payload["constants"] = decodedConstants
	}
	return systemcmd.MarshalJSON(payload)
}
