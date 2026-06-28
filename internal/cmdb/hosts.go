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

// Package cmdb contains reusable CMDB-domain operations shared by CLI surfaces.
package cmdb

import (
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"

	json "github.com/goccy/go-json"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
	"github.com/TencentBlueKing/bk-cli/internal/utils"
)

const GatewayName = "bk-cmdb"

var defaultHostFields = []string{"bk_host_id", "bk_host_innerip", "bk_cloud_id", "bk_host_name"}

const defaultHostsUsageHint = "--hosts 10.0.0.1,27:10.0.0.2"

type HostIP struct {
	IP      string
	CloudID int
}

type Host struct {
	ID      int64
	IP      string
	CloudID int
	Name    string
}

type ResolveBizHostsInput struct {
	BizID   int
	Hosts   string
	Stage   string
	Headers []string
	Limit   int
}

type ResolveBizHostsResult struct {
	Requested []HostIP
	Hosts     []Host
	HostIDs   []int64
	Envelope  *output.Envelope
}

// ParseHostIPs parses the shared CMDB host token grammar into distinct cloud/IP pairs.
func ParseHostIPs(raw string) ([]HostIP, error) {
	return ParseHostIPsWithHint(raw, defaultHostsUsageHint)
}

// ParseHostIPsWithHint parses the shared CMDB host token grammar into distinct cloud/IP pairs.
func ParseHostIPsWithHint(raw, usageHint string) ([]HostIP, error) {
	tokens := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	if len(tokens) == 0 {
		return nil, output.UserError(
			"invalid_argument",
			"host_ips must include at least one host entry",
			"Use "+usageHint,
		)
	}
	hostIPs := make([]HostIP, 0, len(tokens))
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		hostIP, err := parseHostIPToken(token)
		if err != nil {
			return nil, output.UserError(
				"invalid_argument",
				fmt.Sprintf("host_ips contains an invalid host entry %q", token),
				"Use "+usageHint,
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

// BuildHostPropertyFilter builds the CMDB host_property_filter payload for the provided hosts.
func BuildHostPropertyFilter(hostIPs []HostIP) map[string]any {
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
				{"field": "bk_host_innerip", "operator": "in", "value": groups[cloudID]},
				{"field": "bk_cloud_id", "operator": "equal", "value": cloudID},
			},
		}
	}
	rules := make([]map[string]any, 0, len(cloudIDs))
	for _, cloudID := range cloudIDs {
		rules = append(rules, map[string]any{
			"condition": "AND",
			"rules": []map[string]any{
				{"field": "bk_host_innerip", "operator": "in", "value": groups[cloudID]},
				{"field": "bk_cloud_id", "operator": "equal", "value": cloudID},
			},
		})
	}
	return map[string]any{"condition": "OR", "rules": rules}
}

// BuildBizHostsRequest builds the CMDB business host lookup request and returns the parsed host list.
func BuildBizHostsRequest(input ResolveBizHostsInput) (syslib.RequestSpec, []HostIP, error) {
	if err := systemcmd.ValidatePositiveIntFlag("biz", input.BizID); err != nil {
		return syslib.RequestSpec{}, nil, err
	}
	requested, err := ParseHostIPs(input.Hosts)
	if err != nil {
		return syslib.RequestSpec{}, nil, err
	}
	limit := input.Limit
	if limit == 0 {
		limit = 500
	}
	if err := systemcmd.ValidatePositiveIntFlag("limit", limit); err != nil {
		return syslib.RequestSpec{}, nil, err
	}
	bodyJSON, err := BuildHostQueryBody(defaultHostFields, 0, limit, requested, nil)
	if err != nil {
		return syslib.RequestSpec{}, nil, err
	}
	return syslib.RequestSpec{
		GatewayName: GatewayName,
		Method:      "POST",
		Path:        fmt.Sprintf("/api/v3/open/hosts/app/%d/list_hosts", input.BizID),
		BodyJSON:    bodyJSON,
		Headers:     input.Headers,
		Stage:       input.Stage,
		AuthConfig: &syslib.AuthConfig{
			AppVerifiedRequired:        true,
			UserVerifiedRequired:       true,
			ResourcePermissionRequired: true,
		},
	}, requested, nil
}

// BuildHostQueryBody builds the CMDB host list request body shared by command and internal callers.
func BuildHostQueryBody(fields []string, start, limit int, hostIPs []HostIP, extra map[string]any) (string, error) {
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
	if len(hostIPs) > 0 {
		payload["host_property_filter"] = BuildHostPropertyFilter(hostIPs)
	}
	return systemcmd.MarshalJSON(payload)
}

// ResolveBizHosts executes the CMDB business host lookup and validates that every requested host resolved.
func ResolveBizHosts(runtime *syslib.Runtime, input ResolveBizHostsInput) (*ResolveBizHostsResult, error) {
	spec, requested, err := BuildBizHostsRequest(input)
	if err != nil {
		return nil, err
	}
	result, err := syslib.ExecuteRequest(runtime, spec)
	if err != nil {
		return nil, err
	}
	hosts, err := parseHostsEnvelope(result.Envelope)
	if err != nil {
		return nil, err
	}
	hostIDs := make([]int64, 0, len(hosts))
	for _, host := range hosts {
		hostIDs = append(hostIDs, host.ID)
	}
	if len(hosts) == 0 {
		return nil, output.UserError(
			"invalid_argument",
			"no hosts matched the requested hosts",
			"Check --hosts and --biz",
		)
	}
	if len(hosts) != len(requested) {
		return nil, output.UserError(
			"invalid_argument",
			fmt.Sprintf("only resolved %d of %d requested hosts", len(hosts), len(requested)),
			"Check --hosts and rerun without missing hosts",
		)
	}
	return &ResolveBizHostsResult{
		Requested: requested,
		Hosts:     hosts,
		HostIDs:   hostIDs,
		Envelope:  result.Envelope,
	}, nil
}

func parseHostIPToken(token string) (HostIP, error) {
	if strings.Count(token, ":") == 0 {
		if !utils.IsIPv4(token) {
			return HostIP{}, fmt.Errorf("invalid IP")
		}
		return HostIP{IP: token, CloudID: 0}, nil
	}
	if strings.Count(token, ":") != 1 {
		return HostIP{}, fmt.Errorf("invalid cloud:ip format")
	}
	parts := strings.SplitN(token, ":", 2)
	cloudID, err := strconv.Atoi(parts[0])
	if err != nil || cloudID < 0 {
		return HostIP{}, fmt.Errorf("invalid cloud ID")
	}
	if !utils.IsIPv4(parts[1]) {
		return HostIP{}, fmt.Errorf("invalid IP")
	}
	return HostIP{IP: parts[1], CloudID: cloudID}, nil
}

func parseHostsEnvelope(env *output.Envelope) ([]Host, error) {
	if env == nil {
		return nil, fmt.Errorf("cmdb host lookup received an empty response envelope")
	}
	data, err := json.Marshal(env.Data)
	if err != nil {
		return nil, output.SystemError(
			"response_error",
			fmt.Sprintf("failed to marshal cmdb host response: %s", err),
			"",
		)
	}
	var payload struct {
		Info []struct {
			ID      int64  `json:"bk_host_id"`
			IP      string `json:"bk_host_innerip"`
			CloudID int    `json:"bk_cloud_id"`
			Name    string `json:"bk_host_name"`
		} `json:"info"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, output.SystemError(
			"response_error",
			fmt.Sprintf("failed to parse cmdb host response: %s", err),
			"",
		)
	}
	hosts := make([]Host, 0, len(payload.Info))
	seen := make(map[int64]struct{}, len(payload.Info))
	for _, item := range payload.Info {
		if item.ID <= 0 {
			return nil, output.SystemError(
				"response_error",
				"cmdb host lookup response contains a host entry without bk_host_id",
				"",
			)
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		hosts = append(hosts, Host{
			ID:      item.ID,
			IP:      item.IP,
			CloudID: item.CloudID,
			Name:    item.Name,
		})
	}
	return hosts, nil
}
