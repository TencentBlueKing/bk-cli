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
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
	"github.com/TencentBlueKing/bk-cli/internal/utils"
)

const gatewayName = "bk-cmdb"

type hostIP struct {
	IP      string
	CloudID int
}

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
		hostIPs, err := parseHostIPs(hostIPsRaw)
		if err != nil {
			return "", err
		}
		payload["host_property_filter"] = buildHostPropertyFilter(hostIPs)
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
	tokens := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	if len(tokens) == 0 {
		return nil, output.UserError(
			"invalid_argument",
			"host_ips must include at least one host entry",
			"Use --host_ips 10.0.0.1,27:10.0.0.2",
		)
	}

	hostIPs := make([]hostIP, 0, len(tokens))
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		hostIP, err := parseHostIPToken(token)
		if err != nil {
			return nil, output.UserError(
				"invalid_argument",
				fmt.Sprintf("host_ips contains an invalid host entry %q", token),
				"Use --host_ips 10.0.0.1,27:10.0.0.2",
			)
		}
		key := fmt.Sprintf("%d:%s", hostIP.CloudID, hostIP.IP)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		hostIPs = append(hostIPs, hostIP)
	}

	return hostIPs, nil
}

func parseHostIPToken(token string) (hostIP, error) {
	if strings.Count(token, ":") == 0 {
		if !utils.IsIPv4(token) {
			return hostIP{}, fmt.Errorf("invalid IP")
		}
		return hostIP{IP: token, CloudID: 0}, nil
	}
	if strings.Count(token, ":") != 1 {
		return hostIP{}, fmt.Errorf("invalid cloud:ip format")
	}

	parts := strings.SplitN(token, ":", 2)
	cloudID, err := strconv.Atoi(parts[0])
	if err != nil || cloudID < 0 {
		return hostIP{}, fmt.Errorf("invalid cloud ID")
	}
	if !utils.IsIPv4(parts[1]) {
		return hostIP{}, fmt.Errorf("invalid IP")
	}
	return hostIP{IP: parts[1], CloudID: cloudID}, nil
}

func buildHostPropertyFilter(hostIPs []hostIP) map[string]any {
	groups := make(map[int][]string, len(hostIPs))
	for _, hostIP := range hostIPs {
		groups[hostIP.CloudID] = append(groups[hostIP.CloudID], hostIP.IP)
	}

	cloudIDs := make([]int, 0, len(groups))
	for cloudID := range groups {
		cloudIDs = append(cloudIDs, cloudID)
	}
	sort.Ints(cloudIDs)

	if len(cloudIDs) == 1 {
		cloudID := cloudIDs[0]
		return map[string]any{
			"condition": "AND",
			"rules": []map[string]any{
				{
					"field":    "bk_host_innerip",
					"operator": "in",
					"value":    groups[cloudID],
				},
				{
					"field":    "bk_cloud_id",
					"operator": "equal",
					"value":    cloudID,
				},
			},
		}
	}

	rules := make([]map[string]any, 0, len(cloudIDs))
	for _, cloudID := range cloudIDs {
		rules = append(rules, map[string]any{
			"condition": "AND",
			"rules": []map[string]any{
				{
					"field":    "bk_host_innerip",
					"operator": "in",
					"value":    groups[cloudID],
				},
				{
					"field":    "bk_cloud_id",
					"operator": "equal",
					"value":    cloudID,
				},
			},
		})
	}

	return map[string]any{
		"condition": "OR",
		"rules":     rules,
	}
}
