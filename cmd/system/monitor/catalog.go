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

package monitor

import (
	_ "embed"
	"fmt"
	"net/http"
	"sort"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"

	json "github.com/goccy/go-json"
)

// tools.json is a reviewed snapshot of Monitor read-only HTTP operations. Execution still fetches no local
// authority from it: the server validates arguments, resources and permissions on every request.
//
//go:embed catalog/tools.json
var catalogJSON []byte

type catalog struct {
	Version       string            `json:"catalog_version"`
	Tools         []tool            `json:"tools"`
	Actions       map[string]string `json:"actions"`
	ResourceTypes map[string]string `json:"resource_types"`
}

type tool struct {
	Endpoint      monitorapi.Endpoint `json:"endpoint"`
	Module        string              `json:"module"`
	Category      string              `json:"category"`
	Name          string              `json:"name"`
	Short         string              `json:"short"`
	Capabilities  []string            `json:"capabilities"`
	Prerequisites []string            `json:"prerequisites"`
	Params        []param             `json:"params"`
	Permission    permissionSpec      `json:"permission"`
}

// permissionSpec mirrors Unified MCP permission metadata. It explains which IAM actions are involved;
// it is never used as an authorization decision.
type permissionSpec struct {
	Mode   string        `json:"mode"`
	MCP    *iamAction    `json:"mcp,omitempty"`
	Native *nativeAction `json:"native,omitempty"`
}

type iamAction struct {
	SystemID     string `json:"system_id"`
	ActionID     string `json:"action_id"`
	ResourceType string `json:"resource_type"`
	ResourceArg  string `json:"resource_arg,omitempty"`
}

type nativeAction struct {
	iamAction
	Conditional *conditionalAction `json:"conditional,omitempty"`
}

type conditionalAction struct {
	iamAction
	When string `json:"when"`
}

type param struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Items       string   `json:"items,omitempty"`
	Required    bool     `json:"required"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
	TimeUnit    string   `json:"time_unit,omitempty"`
}

type module struct {
	Name        string
	Category    string
	Description string
}

var modules = []module{
	{"space", "metadata", "Find business spaces and BCS clusters"},
	{"metric", "metrics", "Metric tables, metric fields, PromQL and SQL queries"},
	{"log", "log", "Log scenes, index sets, fields, search, context and analysis"},
	{"alert", "alert", "Alerts, alert events, targets and related strategies"},
	{"event", "event", "Event sources and event search"},
	{"apm", "apm", "APM applications, services, traces, spans and profiling"},
	{"dashboard", "dashboard", "Dashboard tree and dashboard details"},
	{"relation", "relation", "Resource relations"},
	{"alarm", "alert_handling", "Alarm strategies, shields, notice groups and action configs (read only)"},
	{"collect", "log_collection", "Log collectors, index-set groups and ETL preview (read only)"},
	{"extract", "log_extract", "Log-extraction topology, hosts, paths and task state (read only)"},
}

func loadCatalog() (*catalog, map[string]tool, error) {
	var value catalog
	if err := json.Unmarshal(catalogJSON, &value); err != nil {
		return nil, nil, fmt.Errorf("invalid monitor tool catalog: %w", err)
	}
	byName := make(map[string]tool, len(value.Tools))
	known := map[string]struct{}{}
	for _, item := range modules {
		known[item.Name] = struct{}{}
	}
	for _, item := range value.Tools {
		if item.Endpoint.Path == "" ||
			(item.Endpoint.Method != http.MethodGet && item.Endpoint.Method != http.MethodPost) ||
			item.Endpoint.Path == "/mcp/execute_tool/" {
			return nil, nil, fmt.Errorf("invalid API route for %q", item.Name)
		}
		if _, ok := known[item.Module]; !ok {
			return nil, nil, fmt.Errorf("monitor tool %q has unknown module %q", item.Name, item.Module)
		}
		if _, exists := byName[item.Name]; exists {
			return nil, nil, fmt.Errorf("monitor tool %q is duplicated", item.Name)
		}
		byName[item.Name] = item
	}
	sort.Slice(value.Tools, func(i, j int) bool { return value.Tools[i].Name < value.Tools[j].Name })
	return &value, byName, nil
}
