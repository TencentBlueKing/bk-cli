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
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

// Profiles contain resource bindings, never credentials, executable templates or grants.
// Keep the format versioned and strict: a typo must not silently broaden a query.
type profileFile struct {
	Version  int                      `yaml:"version" json:"version"`
	Sources  map[string]profileSource `yaml:"sources,omitempty" json:"sources,omitempty"`
	Profiles map[string]queryProfile  `yaml:"profiles" json:"profiles"`
}

type queryProfile struct {
	Description string                      `yaml:"description,omitempty" json:"description,omitempty"`
	Environment string                      `yaml:"environment,omitempty" json:"environment,omitempty"`
	Components  map[string]profileComponent `yaml:"components,omitempty" json:"components,omitempty"`
	Context     string                      `yaml:"context,omitempty" json:"context,omitempty"`
	BizID       string                      `yaml:"bk_biz_id,omitempty" json:"bk_biz_id,omitempty"`
	Log         *logProfile                 `yaml:"log,omitempty" json:"log,omitempty"`
	Metric      *metricProfile              `yaml:"metric,omitempty" json:"metric,omitempty"`
	APM         *apmProfile                 `yaml:"apm,omitempty" json:"apm,omitempty"`
}

type logProfile struct {
	IndexSetID int64             `yaml:"index_set_id,omitempty" json:"index_set_id,omitempty"`
	Scene      string            `yaml:"scene,omitempty" json:"scene,omitempty"`
	Where      map[string]string `yaml:"where,omitempty" json:"where,omitempty"`
	Filters    map[string]string `yaml:"filters,omitempty" json:"filters,omitempty"`
	Conditions map[string]any    `yaml:"conditions,omitempty" json:"conditions,omitempty"`
}

type metricProfile struct {
	TableID string `yaml:"table_id" json:"table_id"`
}

type apmProfile struct {
	AppName     string `yaml:"app_name" json:"app_name"`
	ServiceName string `yaml:"service_name,omitempty" json:"service_name,omitempty"`
}

type selectedProfile struct {
	Name       string            `json:"name"`
	File       string            `json:"file"`
	SHA256     string            `json:"sha256"`
	Profile    queryProfile      `json:"-"`
	Applied    []string          `json:"applied_fields"`
	Resolved   map[string]any    `json:"resolved_parameters"`
	RowFilters map[string]string `json:"-"`
	Version    int               `json:"version"`
	Definition *queryProfile     `json:"-"`
	FileSpec   *profileFile      `json:"-"`
	Component  string            `json:"component,omitempty"`
	Binding    string            `json:"binding,omitempty"`
	Source     string            `json:"source,omitempty"`
	Route      *profileBinding   `json:"-"`
	ToolArgs   map[string]any    `json:"-"`
	Ignored    []string          `json:"ignored_fields,omitempty"`
}

var profileNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

func profileFilePath(explicit string) (string, error) {
	if explicit == "" {
		explicit = os.Getenv("BK_CLI_MONITOR_PROFILE_FILE")
	}
	if explicit == "" {
		explicit = filepath.Join(config.BaseDirectory(), "monitor", "profiles.yaml")
	}
	path, err := filepath.Abs(explicit)
	if err != nil {
		return "", monitorapi.InputError("invalid_profile_file", "无法解析 Profile 文件路径。")
	}
	return path, nil
}

func loadProfiles(path string) (*profileFile, string, error) {
	raw, err := readFileLimited(path)
	if err != nil {
		return nil, "", monitorapi.InputError(
			"invalid_profile_file",
			"Profile 文件不可读、为空或超过 64 KiB；检查 --profile_file。",
		)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	var file profileFile
	// Do not echo YAML parser errors: they may contain values mistakenly used as credentials.
	if err := decoder.Decode(&file); err != nil {
		return nil, "", monitorapi.InputError(
			"invalid_profile_file",
			"Profile YAML 格式、类型或字段不合法；支持 version、profiles 及文档列出的资源字段。",
		)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, "", monitorapi.InputError("invalid_profile_file", "Profile 文件只允许一个 YAML 文档。")
	}
	if (file.Version != 1 && file.Version != 2) || len(file.Profiles) == 0 || len(file.Profiles) > 100 {
		return nil, "", monitorapi.InputError(
			"invalid_profile_file",
			"需要 version: 1 或 2，且 profiles 包含 1–100 个具名配置。",
		)
	}
	if file.Version == 2 {
		if err := validateV2(&file); err != nil {
			return nil, "", err
		}
		return &file, fmt.Sprintf("%x", sha256.Sum256(raw)), nil
	}
	if len(file.Sources) > 0 {
		return nil, "", monitorapi.InputError("invalid_profile_file", "sources 需要 version: 2。")
	}
	for _, name := range sortedProfileNames(&file) {
		if !profileNamePattern.MatchString(name) {
			return nil, "", monitorapi.InputError(
				"invalid_profile_name",
				"Profile 名称限 1–128 个字母、数字、点、下划线或连字符，首字符为字母或数字。",
			)
		}
		if err := validateProfile(file.Profiles[name]); err != nil {
			return nil, "", err
		}
	}
	return &file, fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

func sortedProfileNames(file *profileFile) []string {
	names := make([]string, 0, len(file.Profiles))
	for name := range file.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func validateProfile(p queryProfile) error {
	invalid := func(message string) error { return monitorapi.InputError("invalid_profile", message) }
	if len(p.Components) > 0 || p.Environment != "" {
		return invalid("components/environment 需要 version: 2。")
	}
	if validate.ValidateContextName(p.Context) != nil {
		return invalid("每个 Profile 必须引用合法的 bk-cli context 名称；不允许 URL 或凭据。")
	}
	biz, err := strconv.ParseInt(p.BizID, 10, 64)
	if err != nil || biz == 0 {
		return invalid("Profile 的 bk_biz_id 必须是非零整数（允许负数空间 ID）。")
	}
	if p.Log != nil {
		l := p.Log
		if err := validateLogFilters(l.Filters); err != nil {
			return err
		}
		if (l.IndexSetID > 0) == (l.Scene != "") || l.IndexSetID < 0 {
			return invalid("log 必须二选一：正整数 index_set_id，或 scene；不允许同时配置。")
		}
		if len(l.Where) > 0 && l.Scene == "" {
			return invalid("log.where 只用于 scene 模式；索引集过滤请用 log.conditions。")
		}
		if len(l.Conditions) > 0 && l.IndexSetID == 0 {
			return invalid("log.conditions 只用于 index_set_id 模式。")
		}
		for key, value := range l.Where {
			if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" || key == "scene" {
				return invalid("log.where 需要非空的场景维度和值；scene 请放在 log.scene。")
			}
		}
	}
	if p.Metric != nil && strings.TrimSpace(p.Metric.TableID) == "" {
		return invalid("metric.table_id 不可为空。")
	}
	if p.APM != nil && strings.TrimSpace(p.APM.AppName) == "" {
		return invalid("apm.app_name 不可为空。")
	}
	return nil
}

// profileArgs fills only parameters accepted by this exact tool. Explicit args win,
// but a business or log-mode change must not reuse identifiers from a different scope.
func profileArgs(selection *selectedProfile, item tool, args map[string]any, scene string, where []string) error {
	if selection == nil {
		return nil
	}
	p := selection.Profile
	if value, exists := args["bk_biz_id"]; exists && jsonText(value) != p.BizID {
		return monitorapi.InputError(
			"profile_business_mismatch",
			"显式业务与 Profile 不一致；请选择对应 Profile 或移除 --profile。",
		)
	}
	fill := func(key string, value any) { fillProfileArgument(selection, item, args, key, value) }
	fill("bk_biz_id", p.BizID)
	applyBindingArgs(selection, item, args)
	if p.Log != nil && (item.Module == "log" || item.Name == "get_log_index_set") {
		l := p.Log
		if l.Scene != "" {
			fill("scene", l.Scene)
		}
		switch {
		case l.IndexSetID > 0:
			if scene != "" || args["table_id_conditions"] != nil || args["target_type"] == "scene" {
				return monitorapi.InputError(
					"profile_log_mode_mismatch",
					"索引集 Profile 不能与场景查询混用；请换 Profile 或移除 --profile。",
				)
			}
			fill("index_set_id", l.IndexSetID)
			if item.Name == "search_logs" && len(l.Conditions) > 0 {
				fill("conditions", l.Conditions)
			}
		case l.Scene != "" && hasParam(item, "index_set_id") && !hasParam(item, "table_id_conditions"):
			return monitorapi.InputError(
				"profile_log_mode_mismatch",
				"此工具需要索引集，当前 Profile 是场景模式；不会自动切换数据源。",
			)
		case l.Scene != "" && hasParam(item, "table_id_conditions"):
			if args["index_set_id"] != nil || args["target_type"] == "index_set" {
				return monitorapi.InputError("profile_log_mode_mismatch", "场景 Profile 不能与索引集查询混用。")
			}
			if scene != "" && scene != l.Scene {
				return monitorapi.InputError(
					"profile_log_mode_mismatch",
					"显式 scene 与 Profile 不一致；请换 Profile 或移除 --profile。",
				)
			}
			// Explicit table_id_conditions replace the entire route; do not deep-merge boolean logic.
			if _, exists := args["table_id_conditions"]; exists && (scene != "" || len(where) > 0) {
				return monitorapi.InputError(
					"conflicting_target",
					"显式 table_id_conditions 不能与 --scene/--where 同时使用。",
				)
			}
			if _, exists := args["table_id_conditions"]; !exists {
				conditions, err := sceneConditions(l.Scene, l.Where, where)
				if err != nil {
					return err
				}
				fill("table_id_conditions", conditions)
			}
			fill("target_type", "scene")
		}
	}
	for key, value := range p.moduleDefaults(item.Module) {
		fill(key, value)
	}
	sort.Strings(selection.Applied)
	recordProfileArgs(selection, args)
	return nil
}

func applyBindingArgs(selection *selectedProfile, item tool, args map[string]any) {
	for key, value := range selection.ToolArgs {
		if hasParam(item, key) {
			fillProfileArgument(selection, item, args, key, value)
		} else {
			selection.Ignored = append(selection.Ignored, key)
		}
	}
	sort.Strings(selection.Ignored)
}

func fillProfileArgument(selection *selectedProfile, item tool, args map[string]any, key string, value any) {
	if _, exists := args[key]; exists || !hasParam(item, key) {
		return
	}
	args[key] = value
	if !slices.Contains(selection.Applied, key) {
		selection.Applied = append(selection.Applied, key)
	}
}

func recordProfileArgs(selection *selectedProfile, args map[string]any) {
	if selection == nil {
		return
	}
	sort.Strings(selection.Applied)
	selection.Resolved = map[string]any{}
	if len(selection.RowFilters) > 0 {
		selection.Resolved["log_filters"] = selection.RowFilters
	}
	keys := []string{
		"bk_biz_id", "index_set_id", "table_id", "table_id_conditions",
		"target_type", "conditions", "app_name", "service_name", "service_names",
	}
	for _, key := range keys {
		if value, ok := args[key]; ok {
			selection.Resolved[key] = value
		}
	}
}

func (p queryProfile) moduleDefaults(module string) map[string]any {
	args := map[string]any{}
	if p.Metric != nil && module == "metric" {
		args["table_id"] = p.Metric.TableID
	}
	if p.APM != nil && module == "apm" {
		args["app_name"] = p.APM.AppName
		if p.APM.ServiceName != "" {
			args["service_name"] = p.APM.ServiceName
			args["service_names"] = []any{p.APM.ServiceName}
		}
	}
	return args
}

func sceneConditions(scene string, defaults map[string]string, where []string) ([]any, error) {
	filters := map[string]string{}
	maps.Copy(filters, defaults)
	for _, raw := range where {
		key, value, ok := strings.Cut(raw, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" || value == "" || key == "scene" {
			return nil, monitorapi.InputError("invalid_where", "--where 使用非空 KEY=VALUE；scene 使用 --scene。")
		}
		filters[key] = value
	}
	keys := make([]string, 0, len(filters))
	for key := range filters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	conditions := []any{map[string]any{"field_name": "scene", "value": []any{scene}, "op": "eq"}}
	for _, key := range keys {
		conditions = append(
			conditions,
			map[string]any{"field_name": key, "value": []any{filters[key]}, "op": "eq"},
		)
	}
	return []any{conditions}, nil
}
