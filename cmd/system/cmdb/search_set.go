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
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newSearchSetCmd(deps systemcmd.BuildDeps) *cobra.Command {
	const (
		defaultSupplierAccount = "0"
		defaultFields          = "bk_set_id,bk_set_name"
		defaultLimit           = 500
	)

	var (
		bizID           int
		supplierAccount string
		fieldsCSV       string
		limit           int
		setName         string
		setID           int
		stage           string
		body            string
		headers         []string
	)

	cmd := &cobra.Command{
		Use:   "search_set",
		Short: "Search CMDB sets",
		Long: `Search CMDB sets through the open API.

When --body is not provided, bk-cli synthesizes the POST body from --fields,
--limit, --bk_set_name, and --bk_set_id.`,
		Example: strings.Join([]string{
			"  bk-cli cmdb search_set --bk_biz_id 2",
			"  bk-cli cmdb search_set --bk_biz_id 2 --bk_set_name web",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := systemcmd.ValidatePositiveIntFlag("bk_biz_id", bizID); err != nil {
				return err
			}

			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}
			resolvedSupplierAccount, err := normalizeSupplierAccount(supplierAccount)
			if err != nil {
				return err
			}

			bodyJSON, err := buildSearchSetBody(
				body,
				fieldsCSV,
				limit,
				setName,
				setID,
				cmd.Flags().Changed("bk_set_id"),
			)
			if err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "search_set", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path: fmt.Sprintf(
					"/api/v3/open/set/search/%s/%d",
					url.PathEscape(resolvedSupplierAccount),
					bizID,
				),
				BodyJSON: bodyJSON,
				Headers:  headers,
				Stage:    stage,
				AuthConfig: &syslib.AuthConfig{
					AppVerifiedRequired:        true,
					UserVerifiedRequired:       true,
					ResourcePermissionRequired: true,
				},
			}, nil)
		},
	}

	cmd.Flags().IntVar(&bizID, "bk_biz_id", 0, "Business ID")
	cmd.Flags().StringVar(
		&supplierAccount,
		"supplier_account",
		defaultSupplierAccount,
		"CMDB supplier account path segment",
	)
	cmd.Flags().StringVar(&fieldsCSV, "fields", defaultFields, "Comma-separated set fields to return")
	cmd.Flags().IntVar(&limit, "limit", defaultLimit, "Maximum number of sets to return")
	cmd.Flags().StringVar(&setName, "bk_set_name", "", "Set name filter")
	cmd.Flags().IntVar(&setID, "bk_set_id", 0, "Set ID filter")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}

func buildSearchSetBody(
	bodyOverride, fieldsCSV string,
	limit int,
	setName string,
	setID int,
	setIDProvided bool,
) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}

	condition := map[string]any{}
	if strings.TrimSpace(setName) != "" {
		condition["bk_set_name"] = strings.TrimSpace(setName)
	}
	if setIDProvided {
		if err := systemcmd.ValidatePositiveIntFlag("bk_set_id", setID); err != nil {
			return "", err
		}
		condition["bk_set_id"] = setID
	}

	return buildPagedSearchBody("", fieldsCSV, limit, condition)
}
