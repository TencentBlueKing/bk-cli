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

func newCreateModuleCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		bizID             int
		setID             int
		moduleName        string
		parentID          int
		moduleType        string
		operator          string
		backupOperator    string
		serviceTemplateID int
		serviceCategoryID int
		stage             string
		body              string
		headers           []string
	)

	cmd := &cobra.Command{
		Use:   "create_module",
		Short: "Create a CMDB module",
		Example: strings.Join([]string{
			"  bk-cli cmdb create_module --bk_biz_id 2 --bk_set_id 10 --bk_module_name web",
			"  bk-cli cmdb create_module --bk_biz_id 2 --bk_set_id 10 --bk_module_name web --operator admin",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := systemcmd.ValidatePositiveIntFlag("bk_biz_id", bizID); err != nil {
				return err
			}
			if err := systemcmd.ValidatePositiveIntFlag("bk_set_id", setID); err != nil {
				return err
			}

			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			bodyJSON, err := buildCreateModuleBody(
				body,
				moduleName,
				setID,
				parentID,
				cmd.Flags().Changed("bk_parent_id"),
				moduleType,
				operator,
				backupOperator,
				serviceTemplateID,
				cmd.Flags().Changed("service_template_id"),
				serviceCategoryID,
				cmd.Flags().Changed("service_category_id"),
			)
			if err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "create_module", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        fmt.Sprintf("/api/v3/open/module/%d/%d", bizID, setID),
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
	cmd.Flags().IntVar(&setID, "bk_set_id", 0, "Set ID")
	cmd.Flags().StringVar(&moduleName, "bk_module_name", "", "Module name")
	cmd.Flags().IntVar(&parentID, "bk_parent_id", 0, "Parent node ID; defaults to bk_set_id")
	cmd.Flags().StringVar(&moduleType, "bk_module_type", "", "Module type")
	cmd.Flags().StringVar(&operator, "operator", "", "Primary maintainer")
	cmd.Flags().StringVar(&backupOperator, "bk_bak_operator", "", "Backup maintainer")
	cmd.Flags().IntVar(&serviceTemplateID, "service_template_id", 0, "Service template ID")
	cmd.Flags().IntVar(&serviceCategoryID, "service_category_id", 0, "Service category ID")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}

func buildCreateModuleBody(
	bodyOverride string,
	moduleName string,
	setID int,
	parentID int,
	parentIDProvided bool,
	moduleType string,
	operator string,
	backupOperator string,
	serviceTemplateID int,
	serviceTemplateIDProvided bool,
	serviceCategoryID int,
	serviceCategoryIDProvided bool,
) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}
	if err := systemcmd.ValidatePositiveIntFlag("bk_set_id", setID); err != nil {
		return "", err
	}
	if err := systemcmd.ValidateNonEmptyStringFlag("bk_module_name", moduleName); err != nil {
		return "", err
	}
	if !parentIDProvided {
		parentID = setID
	}
	if err := systemcmd.ValidatePositiveIntFlag("bk_parent_id", parentID); err != nil {
		return "", err
	}
	if serviceTemplateIDProvided && serviceTemplateID < 0 {
		return "", output.UserError(
			"invalid_argument",
			"service_template_id must be greater than or equal to 0",
			"Use --service_template_id 0 or a positive integer",
		)
	}
	if serviceCategoryIDProvided && serviceCategoryID < 0 {
		return "", output.UserError(
			"invalid_argument",
			"service_category_id must be greater than or equal to 0",
			"Use --service_category_id 0 or a positive integer",
		)
	}

	payload := map[string]any{
		"bk_parent_id":   parentID,
		"bk_module_name": strings.TrimSpace(moduleName),
	}
	if strings.TrimSpace(moduleType) != "" {
		payload["bk_module_type"] = strings.TrimSpace(moduleType)
	}
	if strings.TrimSpace(operator) != "" {
		payload["operator"] = strings.TrimSpace(operator)
	}
	if strings.TrimSpace(backupOperator) != "" {
		payload["bk_bak_operator"] = strings.TrimSpace(backupOperator)
	}
	if serviceTemplateIDProvided {
		payload["service_template_id"] = serviceTemplateID
	}
	if serviceCategoryIDProvided {
		payload["service_category_id"] = serviceCategoryID
	}

	return systemcmd.MarshalJSON(payload)
}
