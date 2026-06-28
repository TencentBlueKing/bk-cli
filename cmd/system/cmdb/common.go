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

// Package cmdb implements the CMDB system command and its Go-implemented actions.
package cmdb

import (
	"maps"
	"strings"

	"github.com/spf13/cobra"

	internalcmdb "github.com/TencentBlueKing/bk-cli/internal/cmdb"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
	"github.com/TencentBlueKing/bk-cli/internal/utils"
)

const gatewayName = "bk-cmdb"

type hostIP = internalcmdb.HostIP

type hostListCommandSpec struct {
	Name          string
	Short         string
	Example       string
	RequiresBizID bool
	Fields        []string
	Limit         int
	PathBuilder   func(bizID int) string
}

type transferHostsToSingleTargetSpec struct {
	Name    string
	Short   string
	Path    string
	Example string
}

func normalizeSupplierAccount(raw string) (string, error) {
	supplierAccount := strings.TrimSpace(raw)
	if supplierAccount == "" {
		return "", output.UserError(
			"invalid_argument",
			"supplier_account cannot be empty",
			"Use --supplier_account 0 or another non-empty supplier account",
		)
	}

	return supplierAccount, nil
}

func buildPagedSearchBody(bodyOverride, fieldsCSV string, limit int, condition map[string]any) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}
	if err := systemcmd.ValidatePositiveIntFlag("limit", limit); err != nil {
		return "", err
	}

	fields := utils.ParseCSVFields(fieldsCSV)
	if len(fields) == 0 {
		return "", output.UserError(
			"invalid_argument",
			"fields must include at least one non-empty field",
			"Use --fields bk_biz_id,bk_biz_name",
		)
	}

	payload := map[string]any{
		"fields": fields,
		"page": map[string]int{
			"start": 0,
			"limit": limit,
		},
	}
	if len(condition) > 0 {
		payload["condition"] = condition
	}

	return systemcmd.MarshalJSON(payload)
}

func buildHostQueryBody(
	bodyOverride string,
	fields []string,
	start, limit int,
	hostIPsRaw string,
	extra map[string]any,
) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}
	if err := systemcmd.ValidateNonNegativeIntFlag("start", start); err != nil {
		return "", err
	}
	if err := systemcmd.ValidatePositiveIntFlag("limit", limit); err != nil {
		return "", err
	}

	payload := map[string]any{
		"fields": fields,
		"page": map[string]int{
			"start": start,
			"limit": limit,
		},
	}
	maps.Copy(payload, extra)
	if strings.TrimSpace(hostIPsRaw) != "" {
		hostIPs, err := internalcmdb.ParseHostIPs(hostIPsRaw)
		if err != nil {
			return "", err
		}
		payload["host_property_filter"] = internalcmdb.BuildHostPropertyFilter(hostIPs)
	}

	return systemcmd.MarshalJSON(payload)
}

func buildTransferHostsToSingleTargetBody(bodyOverride string, bizID int, hostIDsCSV string) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}
	if err := systemcmd.ValidatePositiveIntFlag("bk_biz_id", bizID); err != nil {
		return "", err
	}
	hostIDs, err := utils.ParseCSVInts(hostIDsCSV, "bk_host_ids", "Use --bk_host_ids 1,2,3")
	if err != nil {
		return "", err
	}

	return systemcmd.MarshalJSON(map[string]any{
		"bk_biz_id":  bizID,
		"bk_host_id": hostIDs,
	})
}

func newHostListCommand(spec hostListCommandSpec, deps systemcmd.BuildDeps) *cobra.Command {
	var (
		bizID      int
		hostIPsRaw string
		stage      string
		body       string
		headers    []string
	)

	cmd := &cobra.Command{
		Use:     spec.Name,
		Short:   spec.Short,
		Example: spec.Example,
		RunE: func(cmd *cobra.Command, args []string) error {
			if spec.RequiresBizID {
				if err := systemcmd.ValidatePositiveIntFlag("bk_biz_id", bizID); err != nil {
					return err
				}
			}

			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			bodyJSON, err := buildHostQueryBody(
				body,
				spec.Fields,
				0,
				spec.Limit,
				hostIPsRaw,
				nil,
			)
			if err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, spec.Name, syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        spec.PathBuilder(bizID),
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

	if spec.RequiresBizID {
		cmd.Flags().IntVar(&bizID, "bk_biz_id", 0, "Business ID")
	}
	cmd.Flags().StringVar(
		&hostIPsRaw,
		"host_ips",
		"",
		"Comma-separated host IPs; use cloud:ip for non-zero cloud IDs",
	)
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}

func newTransferHostsToSingleTargetCmd(
	spec transferHostsToSingleTargetSpec,
	deps systemcmd.BuildDeps,
) *cobra.Command {
	var (
		bizID      int
		hostIDsCSV string
		stage      string
		body       string
		headers    []string
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

			bodyJSON, err := buildTransferHostsToSingleTargetBody(body, bizID, hostIDsCSV)
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
					UserVerifiedRequired:       true,
					ResourcePermissionRequired: true,
				},
			}, nil)
		},
	}

	cmd.Flags().IntVar(&bizID, "bk_biz_id", 0, "Business ID")
	cmd.Flags().StringVar(&hostIDsCSV, "bk_host_ids", "", "Comma-separated host IDs")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)

	return cmd
}

func parseHostIPs(raw string) ([]hostIP, error) {
	return internalcmdb.ParseHostIPs(raw)
}

func buildHostPropertyFilter(hostIPs []hostIP) map[string]any {
	return internalcmdb.BuildHostPropertyFilter(hostIPs)
}
