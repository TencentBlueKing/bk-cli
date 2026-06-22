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
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newCreateSetCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		bizID         int
		setName       string
		parentID      int
		setTemplateID int
		setEnv        string
		serviceStatus string
		setDesc       string
		capacity      int
		stage         string
		body          string
		headers       []string
	)

	cmd := &cobra.Command{
		Use:   "create_set",
		Short: "Create a CMDB set",
		Example: strings.Join([]string{
			"  bk-cli cmdb create_set --bk_biz_id 2 --bk_set_name web",
			"  bk-cli cmdb create_set --bk_biz_id 2 --bk_set_name web --bk_set_env 3 --bk_service_status 1",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := systemcmd.ValidatePositiveIntFlag("bk_biz_id", bizID); err != nil {
				return err
			}

			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			bodyJSON, err := buildCreateSetBody(
				body,
				bizID,
				setName,
				parentID,
				cmd.Flags().Changed("bk_parent_id"),
				setTemplateID,
				setEnv,
				serviceStatus,
				setDesc,
				capacity,
				cmd.Flags().Changed("bk_capacity"),
			)
			if err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "create_set", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        fmt.Sprintf("/api/v3/open/set/%d", bizID),
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
	cmd.Flags().StringVar(&setName, "bk_set_name", "", "Set name")
	cmd.Flags().IntVar(&parentID, "bk_parent_id", 0, "Parent node ID; defaults to bk_biz_id")
	cmd.Flags().IntVar(&setTemplateID, "set_template_id", 0, "Set template ID")
	cmd.Flags().StringVar(&setEnv, "bk_set_env", "", "Environment type: 1-test, 2-staging, 3-production")
	cmd.Flags().StringVar(&serviceStatus, "bk_service_status", "", "Service status: 1-open, 2-closed")
	cmd.Flags().StringVar(&setDesc, "bk_set_desc", "", "Set description")
	cmd.Flags().IntVar(&capacity, "bk_capacity", 0, "Set capacity")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}

func buildCreateSetBody(
	bodyOverride string,
	bizID int,
	setName string,
	parentID int,
	parentIDProvided bool,
	setTemplateID int,
	setEnv string,
	serviceStatus string,
	setDesc string,
	capacity int,
	capacityProvided bool,
) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}
	if err := systemcmd.ValidatePositiveIntFlag("bk_biz_id", bizID); err != nil {
		return "", err
	}
	if err := systemcmd.ValidateNonEmptyStringFlag("bk_set_name", setName); err != nil {
		return "", err
	}
	if !parentIDProvided {
		parentID = bizID
	}
	if err := systemcmd.ValidatePositiveIntFlag("bk_parent_id", parentID); err != nil {
		return "", err
	}
	if setTemplateID < 0 {
		return "", output.UserError(
			"invalid_argument",
			"set_template_id must be greater than or equal to 0",
			"Use --set_template_id 0 or a positive integer",
		)
	}
	if capacityProvided && capacity < 0 {
		return "", output.UserError(
			"invalid_argument",
			"bk_capacity must be greater than or equal to 0",
			"Use --bk_capacity 0 or a positive integer",
		)
	}

	payload := map[string]any{
		"bk_parent_id":    parentID,
		"bk_set_name":     strings.TrimSpace(setName),
		"set_template_id": setTemplateID,
		"default":         0,
	}
	if strings.TrimSpace(setEnv) != "" {
		payload["bk_set_env"] = strings.TrimSpace(setEnv)
	}
	if strings.TrimSpace(serviceStatus) != "" {
		payload["bk_service_status"] = strings.TrimSpace(serviceStatus)
	}
	if strings.TrimSpace(setDesc) != "" {
		payload["bk_set_desc"] = strings.TrimSpace(setDesc)
	}
	if capacityProvided {
		payload["bk_capacity"] = capacity
	}

	return systemcmd.MarshalJSON(payload)
}
