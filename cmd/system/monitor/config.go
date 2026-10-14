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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TencentBlueKing/bk-cli/internal/api"
	"github.com/TencentBlueKing/bk-cli/internal/config"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
)

// Profiles contain query resources only. Authentication always belongs to bk-cli.
type profile struct {
	DefaultEnv       string              `json:"default_env,omitempty"`
	Environments     map[string]*profile `json:"environments,omitempty"`
	SpaceUID         string              `json:"space_uid,omitempty"`
	LogSpaceUID      string              `json:"log_space_uid,omitempty"`
	MetricsSpaceUID  string              `json:"metrics_space_uid,omitempty"`
	LogTable         string              `json:"log_table_id,omitempty"`
	TraceTable       string              `json:"trace_table_id,omitempty"`
	LogTables        map[string]string   `json:"log_tables,omitempty"`
	MetricsNamespace string              `json:"metrics_namespace,omitempty"`
	Gateway          *gateway            `json:"apigw,omitempty"`
}

type gateway struct {
	Name       string `json:"gateway_name,omitempty"`
	Stage      string `json:"stage,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	RawPath    string `json:"query_raw_path,omitempty"`
	TSPath     string `json:"query_ts_path,omitempty"`
	PromQLPath string `json:"query_promql_path,omitempty"`
	ProxyPath  string `json:"proxy_path,omitempty"`
}

func configPath() string {
	if path := os.Getenv("BK_CLI_MONITOR_CONFIG"); path != "" {
		return path
	}
	return filepath.Join(config.BaseDirectory(), "monitor.json")
}

func readProfiles(path string, required bool) (profile, error) {
	var root profile
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) && !required && os.Getenv("BK_CLI_MONITOR_CONFIG") == "" {
		return root, nil
	}
	if err != nil {
		return root, fmt.Errorf("read monitor configuration: %w", err)
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return root, fmt.Errorf("invalid monitor configuration JSON")
	}
	return root, nil
}

func selectProfile(root profile, name string, defaults bool) (profile, error) {
	if name == "" && defaults {
		name = os.Getenv("BK_CLI_MONITOR_ENV")
		if name == "" {
			name = root.DefaultEnv
		}
	}
	if name == "" {
		return root, nil
	}
	p, ok := root.Environments[name]
	if !ok || p == nil {
		return profile{}, fmt.Errorf("unknown or invalid environment %q", name)
	}
	selected := *p
	if selected.Gateway == nil {
		selected.Gateway = root.Gateway
	}
	return selected, nil
}

func (p profile) space(kind, override string) string {
	if override != "" {
		return override
	}
	if value := os.Getenv("BK_CLI_MONITOR_SPACE_UID"); value != "" {
		return value
	}
	if kind == "logs" && p.LogSpaceUID != "" {
		return p.LogSpaceUID
	}
	if (kind == "metrics" || kind == "query") && p.MetricsSpaceUID != "" {
		return p.MetricsSpaceUID
	}
	return p.SpaceUID
}

func (p profile) request(runtime *syslib.Runtime, kind, stage string, explicitStage bool,
	body map[string]any, headers []string, timeout string,
) (syslib.RequestSpec, error) {
	g := gateway{
		Name: "bk-unify-query", Stage: "prod", RawPath: "/query/ts/raw",
		TSPath: "/query/ts", PromQLPath: "/query/promql",
	}
	if p.Gateway != nil {
		configured := *p.Gateway
		if configured.Name != "" {
			g.Name = configured.Name
		}
		if configured.Stage != "" {
			g.Stage = configured.Stage
		}
		if configured.RawPath != "" {
			g.RawPath = configured.RawPath
		}
		if configured.TSPath != "" {
			g.TSPath = configured.TSPath
		}
		if configured.PromQLPath != "" {
			g.PromQLPath = configured.PromQLPath
		}
		g.ProxyPath = configured.ProxyPath
		if configured.BaseURL != "" {
			// Resource URLs must match the selected deployment context.
			expected := strings.TrimRight(configured.BaseURL, "/")
			if configured.Stage == "" {
				g.Stage = expected[strings.LastIndex(expected, "/")+1:]
			}
			base, err := api.BuildURL(runtime.Config.BkAPIURLTmpl, g.Name, g.Stage, "")
			if err != nil || strings.TrimRight(base, "/") != expected {
				return syslib.RequestSpec{}, fmt.Errorf(
					"apigw.base_url does not match the selected bk-cli context; configure context or apigw.gateway_name",
				)
			}
		}
	}
	if explicitStage {
		g.Stage = stage
	}
	path := g.RawPath
	if kind == "metrics" {
		path = g.TSPath
	}
	if kind == "query" {
		path = g.PromQLPath
	}
	if g.ProxyPath != "" {
		internalPath := "/query/ts/raw"
		if kind == "metrics" {
			internalPath = "/query/ts"
		}
		if kind == "query" {
			internalPath = "/query/ts/promql"
		}
		path = g.ProxyPath
		body = map[string]any{"path": internalPath, "data": body}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return syslib.RequestSpec{}, err
	}
	return syslib.RequestSpec{
		GatewayName: g.Name, Stage: g.Stage, Method: "POST", Path: path,
		BodyJSON: string(encoded), Headers: headers, Timeout: timeout,
		AuthConfig: &syslib.AuthConfig{AppVerifiedRequired: true, ResourcePermissionRequired: true},
	}, nil
}
