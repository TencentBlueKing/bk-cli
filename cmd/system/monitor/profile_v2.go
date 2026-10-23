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
	"maps"
	"slices"
	"sort"
	"strings"

	json "github.com/goccy/go-json"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

// V2 separates the observed system/component from the actual evidence source.
// Native tool arguments remain native; this is not a template or workflow engine.
type profileSource struct {
	Context string `yaml:"context" json:"context"`
	BizID   string `yaml:"bk_biz_id" json:"bk_biz_id"`
}

type profileComponent struct {
	Description string                    `yaml:"description,omitempty" json:"description,omitempty"`
	Bindings    map[string]profileBinding `yaml:"bindings" json:"bindings"`
}

type profileBinding struct {
	Description string                    `yaml:"description,omitempty" json:"description,omitempty"`
	Module      string                    `yaml:"module" json:"module"`
	Source      string                    `yaml:"source" json:"source"`
	Args        map[string]any            `yaml:"args,omitempty" json:"args,omitempty"`
	Tools       map[string]map[string]any `yaml:"tools,omitempty" json:"tools,omitempty"`
	Options     *logBindingOptions        `yaml:"options,omitempty" json:"options,omitempty"`
}

type logBindingOptions struct {
	Scene   string            `yaml:"scene,omitempty" json:"scene,omitempty"`
	Where   map[string]string `yaml:"where,omitempty" json:"where,omitempty"`
	Filters map[string]string `yaml:"filters,omitempty" json:"filters,omitempty"`
}

var bindingResourceArgs = map[string]bool{
	"index_set_id": true, "table_id": true, "result_table_id": true, "app_name": true,
	"service_name": true, "service_names": true, "dashboard_uid": true, "collector_config_id": true,
}

func validateV2(file *profileFile) error {
	bad := func(message string) error { return monitorapi.InputError("invalid_profile_v2", message) }
	if len(file.Sources) == 0 || len(file.Sources) > 100 {
		return bad("version: 2 需要 1–100 个具名 sources。")
	}
	for name, source := range file.Sources {
		if !profileNamePattern.MatchString(name) {
			return bad("source 名称不合法。")
		}
		if validate.ValidateContextName(source.Context) != nil {
			return bad("source.context 必须是已有 context 的合法名称，不接受 URL 或凭据。")
		}
		if err := validateProfile(queryProfile{Context: source.Context, BizID: source.BizID}); err != nil {
			return err
		}
	}
	for _, name := range sortedProfileNames(file) {
		p := file.Profiles[name]
		if !profileNamePattern.MatchString(name) || len(p.Components) == 0 || len(p.Components) > 100 {
			return bad("V2 Profile 需要合法名称与 1–100 个 components。")
		}
		if p.Context != "" || p.BizID != "" || p.Log != nil || p.Metric != nil || p.APM != nil {
			return bad("V2 Profile 不混用 V1 的 context/bk_biz_id/log/metric/apm；数据来源放在 sources，资源放在 bindings。")
		}
		for componentName, component := range p.Components {
			if !profileNamePattern.MatchString(componentName) || len(component.Bindings) == 0 ||
				len(component.Bindings) > 100 {
				return bad("component 需要合法名称与 1–100 个 bindings。")
			}
			for bindingName, binding := range component.Bindings {
				if !profileNamePattern.MatchString(bindingName) {
					return bad("binding 名称不合法。")
				}
				if _, ok := file.Sources[binding.Source]; !ok {
					return bad("binding 引用了不存在的 source。")
				}
				if moduleByName(binding.Module).Name == "" {
					return bad("binding.module 必须是现有的 monitor 业务模块。")
				}
				if err := validateBinding(&binding); err != nil {
					return err
				}
				component.Bindings[bindingName] = binding
			}
		}
	}
	return nil
}

func validateBinding(b *profileBinding) error {
	bad := func(message string) error { return monitorapi.InputError("invalid_profile_binding", message) }
	var err error
	b.Args, err = normalizeProfileArgs(b.Args)
	if err != nil {
		return err
	}
	for key, value := range b.Args {
		if !bindingResourceArgs[key] {
			return bad("binding.args 只放公共资源定位字段；查询、条件、id 等放在 tools.<精确工具名> 中。")
		}
		found := false
		for _, item := range catalogData.Tools {
			if item.Module != b.Module {
				continue
			}
			for _, p := range item.Params {
				if p.Name == key {
					found = true
					if err := validatePresetValue(p, value); err != nil {
						return err
					}
				}
			}
		}
		if !found {
			return bad("binding.args 字段不属于该 module。")
		}
	}
	if err := validateBindingTools(b); err != nil {
		return err
	}
	if b.Options != nil {
		if b.Module != "log" {
			return bad("options.scene/where/filters 仅适用于 log；其他模块用工具原始参数。")
		}
		if err := validateLogFilters(b.Options.Filters); err != nil {
			return err
		}
		if b.Options.Scene == "" && len(b.Options.Where) > 0 {
			return bad("options.where 需要 options.scene。")
		}
		for k, v := range b.Options.Where {
			if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" || k == "scene" {
				return bad("options.where 需要非空路由标签和值。")
			}
		}
	}
	// Validate mode conflicts for both the base binding and every per-tool override.
	for _, name := range append([]string{""}, sortedKeys(b.Tools)...) {
		if _, err := bindingLog(*b, bindingArgs(*b, name)); err != nil {
			return err
		}
	}
	return nil
}

func validateBindingTools(b *profileBinding) error {
	bad := func(message string) error { return monitorapi.InputError("invalid_profile_binding", message) }
	var err error
	for name, values := range b.Tools {
		item, ok := toolsByName[name]
		if !ok || item.Module != b.Module {
			return bad("tools 只能引用当前 module 的已审查只读工具。")
		}
		values, err = normalizeProfileArgs(values)
		if err != nil {
			return err
		}
		for key, value := range values {
			if key == "bk_biz_id" || key == "start_time" || key == "end_time" || key == "timestamp" ||
				key == "query_list" {
				return bad("业务来自 source；时间和关联批次 query_list 必须在运行时显式提供，不能固定在 Profile。")
			}
			found := false
			for _, p := range item.Params {
				if p.Name == key {
					found = true
					if err := validatePresetValue(p, value); err != nil {
						return err
					}
				}
			}
			if !found {
				return bad("tools 中的参数不属于该精确工具。")
			}
		}
		b.Tools[name] = values
	}
	return nil
}

func normalizeProfileArgs(values map[string]any) (map[string]any, error) {
	if values == nil {
		return map[string]any{}, nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return nil, monitorapi.InputError("invalid_profile_args", "Profile 参数必须是 JSON 兼容值。")
	}
	var result map[string]any
	if err := decodeJSON(raw, &result); err != nil {
		return nil, monitorapi.InputError("invalid_profile_args", "无法解析 Profile 参数。")
	}
	return result, nil
}

func validatePresetValue(p param, value any) error {
	valid := false
	switch p.Type {
	case "string":
		_, valid = value.(string)
	case "integer":
		n, ok := value.(json.Number)
		if ok {
			_, err := n.Int64()
			valid = err == nil
		}
	case "number":
		_, valid = value.(json.Number)
	case "boolean":
		_, valid = value.(bool)
	case "array":
		items, ok := value.([]any)
		valid = ok
		if ok && isScalar(p.Items) {
			for _, item := range items {
				if err := validatePresetValue(param{Name: p.Name, Type: p.Items}, item); err != nil {
					return err
				}
			}
		}
	case "object":
		_, valid = value.(map[string]any)
	}
	if !valid {
		return monitorapi.InputError(
			"invalid_profile_args",
			"参数 "+p.Name+" 的类型必须是 "+p.Type+"；完整 Schema 仍由服务端校验。",
		)
	}
	if len(p.Enum) > 0 {
		text, ok := value.(string)
		if ok {
			if slices.Contains(p.Enum, text) {
				return nil
			}
			return monitorapi.InputError("invalid_profile_args", "参数 "+p.Name+" 不在允许值中。")
		}
	}
	return nil
}

func bindingArgs(binding profileBinding, name string) map[string]any {
	values := map[string]any{}
	maps.Copy(values, binding.Args)
	maps.Copy(values, binding.Tools[name])
	return values
}

func bindingLog(binding profileBinding, values map[string]any) (*logProfile, error) {
	if binding.Module != "log" {
		return nil, nil
	}
	l := &logProfile{}
	if raw, ok := values["index_set_id"]; ok {
		id, valid := exactInteger(raw)
		if !valid || id <= 0 {
			return nil, monitorapi.InputError("invalid_profile_binding", "index_set_id 必须是正整数。")
		}
		l.IndexSetID = id
	}
	if binding.Options != nil {
		l.Scene = binding.Options.Scene
		l.Where = binding.Options.Where
		l.Filters = binding.Options.Filters
	}
	if l.IndexSetID > 0 && l.Scene != "" {
		return nil, monitorapi.InputError("profile_log_mode_mismatch", "同一 binding 不可同时指定索引集和场景。")
	}
	return l, nil
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
