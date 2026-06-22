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

	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
	"github.com/TencentBlueKing/bk-cli/internal/utils"
)

func newSearchBusinessCmd(deps systemcmd.BuildDeps) *cobra.Command {
	const (
		defaultSupplierAccount = "0"
		defaultFields          = "bk_biz_id,bk_biz_name,bk_biz_maintainer,bk_biz_productor"
		defaultLimit           = 500
	)

	var (
		bizID           int
		bizIDsCSV       string
		fieldsCSV       string
		limit           int
		supplierAccount string
		stage           string
		body            string
		headers         []string
	)

	cmd := &cobra.Command{
		Use:   "search_business",
		Short: "Search CMDB businesses",
		Long: `Search CMDB businesses through the open API.

By default, bk-cli synthesizes the POST body from flag inputs.
When --body is not provided, you must supply one of --bk_biz_id or --bk_biz_ids.
If --body is provided, bk-cli sends that JSON body unchanged and ignores
the synthesized body inputs such as --bk_biz_id, --bk_biz_ids, --fields, and --limit.`,
		Example: strings.Join([]string{
			"  bk-cli cmdb search_business --bk_biz_id 2",
			"  bk-cli cmdb search_business --bk_biz_ids 1,2,3",
			"  bk-cli cmdb search_business --bk_biz_ids 1,2,3 --fields bk_biz_id,bk_biz_name",
			"  bk-cli cmdb search_business --body '" +
				"{\"condition\":{\"bk_biz_id\":1},\"fields\":[\"bk_biz_id\",\"bk_biz_name\"],\"page\":{\"start\":0,\"limit\":10}}'",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			resolvedSupplierAccount, err := normalizeSupplierAccount(supplierAccount)
			if err != nil {
				return err
			}

			bodyJSON, err := buildSearchBusinessBody(
				body,
				fieldsCSV,
				limit,
				bizID,
				cmd.Flags().Changed("bk_biz_id"),
				bizIDsCSV,
			)
			if err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "search_business", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path: fmt.Sprintf(
					"/api/v3/open/biz/search/%s/",
					url.PathEscape(resolvedSupplierAccount),
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

	cmd.Flags().IntVar(&bizID, "bk_biz_id", 0, "Single business ID filter")
	cmd.Flags().StringVar(&bizIDsCSV, "bk_biz_ids", "", "Comma-separated business ID filters")
	cmd.Flags().StringVar(
		&fieldsCSV,
		"fields",
		defaultFields,
		"Comma-separated business fields to return",
	)
	cmd.Flags().IntVar(&limit, "limit", defaultLimit, "Maximum number of businesses to return")
	cmd.Flags().StringVar(
		&supplierAccount,
		"supplier_account",
		defaultSupplierAccount,
		"CMDB supplier account path segment",
	)
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)
	cmd.MarkFlagsMutuallyExclusive("bk_biz_id", "bk_biz_ids")

	return cmd
}

func buildSearchBusinessBody(
	bodyOverride, fieldsCSV string,
	limit, bizID int,
	bizIDProvided bool,
	bizIDsCSV string,
) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}
	if err := systemcmd.ValidatePositiveIntFlag("limit", limit); err != nil {
		return "", err
	}
	if bizIDProvided {
		if err := systemcmd.ValidatePositiveIntFlag("bk_biz_id", bizID); err != nil {
			return "", err
		}
	}
	if !bizIDProvided && strings.TrimSpace(bizIDsCSV) == "" {
		return "", output.UserError(
			"invalid_argument",
			"one of bk_biz_id or bk_biz_ids is required when --body is not provided",
			"Use --bk_biz_id 2, --bk_biz_ids 1,2,3, or provide an explicit --body",
		)
	}

	condition := map[string]any{}
	if bizIDProvided {
		condition["bk_biz_id"] = bizID
	}
	if strings.TrimSpace(bizIDsCSV) != "" {
		bizIDs, err := utils.ParseCSVInts(bizIDsCSV, "bk_biz_ids", "Use --bk_biz_ids 1,2,3")
		if err != nil {
			return "", err
		}
		condition["bk_biz_id"] = map[string][]int{"$in": bizIDs}
	}

	return buildPagedSearchBody("", fieldsCSV, limit, condition)
}
