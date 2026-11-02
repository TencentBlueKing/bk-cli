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
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func profileInspection(cmd *cobra.Command) bool {
	return cmd.Name() == "status" || cmd.Name() == "show" ||
		(cmd.Name() == "check" && cmd.Flags().Lookup("tool") == nil)
}

func requestedProfileModule(cmd *cobra.Command) (string, string) {
	if item, ok := toolsByName[cmd.Name()]; ok {
		return item.Module, item.Name
	}
	if name, _ := cmd.Flags().GetString("tool"); name != "" {
		if item, ok := toolsByName[name]; ok {
			return item.Module, name
		}
	}
	for _, flag := range []string{"module", "category"} {
		if value, _ := cmd.Flags().GetString(flag); value != "" {
			for _, m := range modules {
				if m.Name == value || m.Category == value {
					return m.Name, ""
				}
			}
		}
	}
	return "", ""
}

func selectV2Profile(cmd *cobra.Command, p *selectedProfile) error {
	component, _ := cmd.Flags().GetString("component")
	binding, _ := cmd.Flags().GetString("binding")
	if component == "" && len(p.Definition.Components) == 1 {
		component = sortedKeys(p.Definition.Components)[0]
	}
	if component == "" {
		if profileInspection(cmd) && binding == "" {
			return nil
		}
		return monitorapi.InputError(
			"needs_component",
			"请用 --component 选择业务组件："+strings.Join(sortedKeys(p.Definition.Components), ", "),
		)
	}
	c, ok := p.Definition.Components[component]
	if !ok {
		return monitorapi.InputError("component_not_found", "Profile 中不存在该 component。")
	}
	p.Component = component
	module, toolName := requestedProfileModule(cmd)
	if binding == "" {
		if profileInspection(cmd) {
			return nil
		}
		candidates := []string{}
		for _, name := range sortedKeys(c.Bindings) {
			if module == "" || c.Bindings[name].Module == module {
				candidates = append(candidates, name)
			}
		}
		if len(candidates) == 0 {
			return monitorapi.InputError("profile_binding_missing", "该组件没有此 module 的绑定；不会回退到其他组件或当前默认环境。")
		}
		if len(candidates) != 1 {
			return monitorapi.InputError(
				"needs_binding",
				"请用 --binding 明确选择："+strings.Join(candidates, ", "),
			)
		}
		binding = candidates[0]
	}
	b, ok := c.Bindings[binding]
	if !ok {
		return monitorapi.InputError("binding_not_found", "该 component 中不存在此 binding。")
	}
	if module != "" && module != b.Module {
		return monitorapi.InputError("profile_binding_mismatch", "binding.module 与当前命令不一致；不会切换到其他能力。")
	}
	if err := activateBinding(p, component, binding, toolName); err != nil {
		return err
	}
	// Business-scoped discovery must stay on this binding's module, not enumerate other sources.
	for _, flagName := range []string{"module", "category"} {
		if flag := cmd.Flags().Lookup(flagName); flag != nil && !flag.Changed {
			value := b.Module
			if flagName == "category" {
				value = categoryFor(value)
			}
			if err := flag.Value.Set(value); err != nil {
				return err
			}
		}
	}
	return nil
}

func activateBinding(p *selectedProfile, component, binding, toolName string) error {
	b := p.Definition.Components[component].Bindings[binding]
	source := p.FileSpec.Sources[b.Source]
	values := bindingArgs(b, toolName)
	log, err := bindingLog(b, values)
	if err != nil {
		return err
	}
	p.Component, p.Binding, p.Source, p.Route = component, binding, b.Source, &b
	p.Profile = queryProfile{Context: source.Context, BizID: source.BizID, Log: log}
	p.ToolArgs = values
	p.Applied = []string{}
	p.Ignored = nil
	p.Resolved = nil
	p.RowFilters = nil
	return nil
}

func bindProfileRuntime(cmd *cobra.Command, deps systemcmd.BuildDeps, p *selectedProfile) (systemcmd.BuildDeps, error) {
	if deps.GetContext != nil && deps.GetContext() != "" && deps.GetContext() != p.Profile.Context {
		return deps, monitorapi.InputError(
			"profile_context_mismatch",
			"--context 与所选绑定的 context 不一致；不会跨环境复用资源 ID。",
		)
	}
	if flag := cmd.Flags().Lookup("bk_biz_id"); flag != nil {
		if flag.Changed && flag.Value.String() != p.Profile.BizID {
			return deps, monitorapi.InputError("profile_business_mismatch", "--bk_biz_id 与所选绑定的数据来源业务不一致。")
		}
		if !flag.Changed {
			if err := flag.Value.Set(p.Profile.BizID); err != nil {
				return deps, err
			}
			p.Applied = append(p.Applied, "bk_biz_id")
			p.Resolved = map[string]any{"bk_biz_id": p.Profile.BizID}
		}
	}
	deps.GetContext = func() string { return p.Profile.Context }
	return deps, nil
}

func cloneBinding(
	cmd *cobra.Command,
	p *selectedProfile,
	component, binding, toolName string,
) (*selectedProfile, error) {
	copy := *p
	if err := activateBinding(&copy, component, binding, toolName); err != nil {
		return nil, err
	}
	cmd.SetContext(context.WithValue(cmd.Context(), profileContextKey{}, &copy))
	return &copy, nil
}

func v2ProfileView(p *selectedProfile) map[string]any {
	components := map[string]any{}
	for _, name := range sortedKeys(p.Definition.Components) {
		if p.Component != "" && p.Component != name {
			continue
		}
		c := p.Definition.Components[name]
		bindings := map[string]any{}
		for _, key := range sortedKeys(c.Bindings) {
			if p.Binding != "" && p.Binding != key {
				continue
			}
			b := c.Bindings[key]
			bindings[key] = map[string]any{
				"module": b.Module, "source": b.Source, "location": p.FileSpec.Sources[b.Source],
				"description": b.Description, "args": b.Args, "tools": b.Tools, "options": b.Options,
			}
		}
		components[name] = map[string]any{"description": c.Description, "bindings": bindings}
	}
	return map[string]any{
		"version":     2,
		"profile":     p.Name,
		"environment": p.Definition.Environment,
		"description": p.Definition.Description,
		"components":  components,
	}
}
