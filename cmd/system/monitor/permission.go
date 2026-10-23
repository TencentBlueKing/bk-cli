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
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

const permissionQueryLimit = 20

func newPermissionSpec() systemcmd.SystemSpec {
	return systemcmd.SystemSpec{
		Name:        "permission",
		Description: "List your permission states, explain one tool, or get IAM apply links",
		RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
			parent.AddCommand(
				newPermissionListCmd(deps),
				newPermissionCheckCmd(deps),
				newPermissionApplyCmd(deps),
			)
			return nil
		},
	}
}

func newPermissionListCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var biz, moduleName string
	var missingOnly bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your permission state for one business, optionally one module",
		Long: `List your own permission state for one business.

This is a read-only view. requires_resource means a concrete resource is still
needed; it is neither denial nor grant. Use permission check for one resource.`,
		Example: "  bk-cli monitor permission list --bk_biz_id <bk_biz_id>\n" +
			"  bk-cli monitor permission list --bk_biz_id <bk_biz_id> --module log --missing_only",
		Args: cobra.NoArgs,
		RunE: runMonitor(deps, func(cmd *cobra.Command, rt *monitorapi.Runtime) error {
			if err := monitorapi.RequireBiz(biz); err != nil {
				return err
			}
			names, err := moduleNames(moduleName)
			if err != nil {
				return err
			}
			rows, summary := []map[string]any{}, map[string]int{}
			for _, current := range names {
				item := moduleByName(current)
				reply, err := monitorapi.Call(rt, "lookup_tool", map[string]any{
					"category":       item.Category,
					"bk_biz_id":      biz,
					"available_only": false,
					"page_size":      100,
				})
				if err != nil {
					return err
				}
				if reply.IsDryRun() {
					return printReply(cmd, reply, nil)
				}
				for _, value := range monitorapi.List(monitorapi.Map(reply.Data)["tools"]) {
					card := monitorapi.Map(value)
					name, _ := card["name"].(string)
					if _, ok := toolsByName[name]; !ok {
						continue
					}
					state, _ := card["permission_state"].(string)
					summary[state]++
					if missingOnly && state != "missing" {
						continue
					}
					rows = append(
						rows,
						permissionRow(name, state, biz, rt.ContextName, currentProfile(cmd)),
					)
				}
			}
			sort.Slice(
				rows,
				func(i, j int) bool { return jsonText(rows[i]["command"]) < jsonText(rows[j]["command"]) },
			)
			data := map[string]any{"bk_biz_id": biz, "summary": summary, "tools": rows}
			if summary["missing"] > 0 {
				commands := []string{}
				for _, row := range rows {
					if row["state"] == "missing" {
						commands = append(commands, jsonText(row["next"]))
					}
				}
				data["next"] = commands
			}
			return writeMonitorEnvelope(cmd, &output.Envelope{OK: true, Data: data})
		}),
	}
	addProfileFlags(cmd)
	cmd.Flags().StringVar(&biz, "bk_biz_id", "", "Business ID")
	cmd.Flags().StringVar(&moduleName, "module", "", "Module: "+moduleChoices())
	cmd.Flags().BoolVar(&missingOnly, "missing_only", false, "Show only tools explicitly denied")
	return cmd
}

func newPermissionCheckCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var biz, name, body string
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check whether one tool can run without reading data",
		Long: `Run lookup_tool, lookup_tool_schema and lookup_permissions.

Only resource fields declared by the tool permission contract are sent. A ready
result is not a cached grant; the real command is authorized again by bk-monitor.`,
		Example: "  bk-cli monitor permission check --bk_biz_id <bk_biz_id> --tool get_index_set_fields\n" +
			"  bk-cli monitor permission check --bk_biz_id <bk_biz_id> --tool get_index_set_fields" +
			" --body '{\"index_set_id\":123}'",
		Args: cobra.NoArgs,
		RunE: runMonitor(deps, func(cmd *cobra.Command, rt *monitorapi.Runtime) error {
			values, err := permissionTarget(cmd, biz, name, body)
			if err != nil {
				return err
			}
			result, err := check(rt, biz, name, values)
			if err != nil {
				return err
			}
			return writeMonitorEnvelope(cmd, &output.Envelope{OK: true, Data: result})
		}),
	}
	addProfileFlags(cmd)
	addPermissionTargetFlags(cmd, &biz, &name, &body)
	return cmd
}

func newPermissionApplyCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var biz, name, moduleName, body string
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Get IAM apply links for missing permissions",
		Long: `Get apply links for permissions that bk-monitor explicitly reports as missing.

This command does not submit an application and does not grant permission. Open
one link in a browser and confirm the request in IAM. Each tool accepts either
the MCP permission or the native SaaS permission.`,
		Example: "  bk-cli monitor permission apply --bk_biz_id <bk_biz_id> --tool list_alerts\n" +
			"  bk-cli monitor permission apply --bk_biz_id <bk_biz_id> --module alert",
		Args: cobra.NoArgs,
		RunE: runMonitor(deps, func(cmd *cobra.Command, rt *monitorapi.Runtime) error {
			if (name == "") == (moduleName == "") {
				return monitorapi.InputError("invalid_target", "--tool 与 --module 二选一。")
			}
			if name != "" {
				values, err := permissionTarget(cmd, biz, name, body)
				if err != nil {
					return err
				}
				return applyTool(cmd, rt, biz, name, values)
			}
			if body != "" {
				return monitorapi.InputError("body_needs_tool", "--body 只用于单个 --tool 的资源上下文。")
			}
			return applyModule(cmd, rt, biz, moduleName)
		}),
	}
	addProfileFlags(cmd)
	addPermissionTargetFlags(cmd, &biz, &name, &body)
	cmd.Flags().StringVar(&moduleName, "module", "", "Module: "+moduleChoices())
	return cmd
}

func applyTool(cmd *cobra.Command, rt *monitorapi.Runtime, biz, name string, values map[string]any) error {
	result, err := check(rt, biz, name, values)
	if err == nil {
		result["applications"] = []any{}
		result["meaning"] = "服务端判定已有权限，无需申请。"
		return writeMonitorEnvelope(cmd, &output.Envelope{OK: true, Data: result})
	}
	decision, ok := monitorapi.IsJudgment(err)
	if !ok || decision.Code != "permission_missing" {
		return err
	}
	return writeMonitorEnvelope(cmd, &output.Envelope{OK: true, Data: map[string]any{
		"decision":     "apply_required",
		"bk_biz_id":    biz,
		"tool":         name,
		"request_id":   decision.RequestID,
		"applications": []any{map[string]any{"tool": name, "any_of": decision.AnyOf}},
		"next":         "在浏览器打开任一 apply_url 并在 IAM 确认申请；审批通过后重新执行原命令。",
	}})
}

func applyModule(cmd *cobra.Command, rt *monitorapi.Runtime, biz, moduleName string) error {
	if err := monitorapi.RequireBiz(biz); err != nil {
		return err
	}
	if _, err := moduleNames(moduleName); err != nil {
		return err
	}
	item := moduleByName(moduleName)
	reply, err := monitorapi.Call(rt, "lookup_tool", map[string]any{
		"category": item.Category, "bk_biz_id": biz, "available_only": false, "page_size": 100,
	})
	if err != nil || reply.IsDryRun() {
		return printReply(cmd, reply, err)
	}
	var missing, unresolved []string
	for _, value := range monitorapi.List(monitorapi.Map(reply.Data)["tools"]) {
		card := monitorapi.Map(value)
		name, _ := card["name"].(string)
		if _, ok := toolsByName[name]; !ok {
			continue
		}
		switch card["permission_state"] {
		case "missing":
			missing = append(missing, name)
		case "requires_resource":
			unresolved = append(unresolved, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(unresolved)
	if len(missing) > permissionQueryLimit {
		return monitorapi.InputError("too_many_missing", "缺权工具超过 20 个；请缩小到单个 --tool。")
	}
	applications := []any{}
	for _, name := range missing {
		_, err := check(rt, biz, name, map[string]any{})
		if decision, ok := monitorapi.IsJudgment(err); ok && decision.Code == "permission_missing" {
			applications = append(
				applications,
				map[string]any{
					"tool":       name,
					"request_id": decision.RequestID,
					"any_of":     decision.AnyOf,
				},
			)
		} else if err != nil {
			return err
		}
	}
	data := map[string]any{
		"bk_biz_id": biz, "module": moduleName, "applications": applications,
		"requires_resource": unresolved,
	}
	if len(applications) == 0 {
		data["decision"] = "no_business_level_application"
		data["meaning"] = "该模块没有业务级缺权。requires_resource 工具需要按具体资源运行 permission apply --tool。"
	} else {
		data["decision"] = "apply_required"
		data["next"] = "在浏览器打开任一 apply_url 并在 IAM 确认申请；审批通过后重新执行原命令。"
	}
	return writeMonitorEnvelope(cmd, &output.Envelope{OK: true, Data: data})
}

func permissionRow(name, state, biz, contextName string, p *selectedProfile) map[string]any {
	row := map[string]any{"tool": name, "command": commandName(name), "state": state}
	switch state {
	case "missing":
		row["next"] = permissionHint(contextName, "apply", name, biz, nil) + profileSuffix(p)
	case "requires_resource":
		row["next"] = permissionHint(contextName, "check", name, biz, nil) + profileSuffix(p)
	}
	return row
}

func permissionTarget(cmd *cobra.Command, biz, name, body string) (map[string]any, error) {
	if err := monitorapi.RequireBiz(biz); err != nil {
		return nil, err
	}
	if _, ok := toolsByName[name]; !ok {
		return nil, monitorapi.InputError(
			"invalid_tool",
			"未知的只读工具；先运行 bk-cli monitor permission list --bk_biz_id "+biz+"。",
		)
	}
	values := map[string]any{}
	if body != "" {
		raw, err := readBody(cmd, body)
		if err != nil {
			return nil, err
		}
		if err := jsonObject(raw, &values); err != nil {
			return nil, err
		}
	}
	if err := profileArgs(currentProfile(cmd), toolsByName[name], values, "", nil); err != nil {
		return nil, err
	}
	if value, exists := values["bk_biz_id"]; exists && jsonText(value) != biz {
		return nil, monitorapi.InputError("conflicting_business", "--body 的业务与 --bk_biz_id 不一致。")
	}
	return values, nil
}

func addPermissionTargetFlags(cmd *cobra.Command, biz, name, body *string) {
	cmd.Flags().StringVar(biz, "bk_biz_id", "", "Business ID")
	cmd.Flags().StringVar(name, "tool", "", "Monitor API tool name")
	cmd.Flags().StringVar(body, "body", "", "Resource fields as JSON, @file, or -; only permission fields are sent")
}

func moduleNames(value string) ([]string, error) {
	if value == "" {
		names := make([]string, 0, len(modules))
		for _, item := range modules {
			names = append(names, item.Name)
		}
		return names, nil
	}
	if moduleByName(value).Name == "" {
		return nil, monitorapi.InputError("invalid_module", "--module 可选："+moduleChoices())
	}
	return []string{value}, nil
}

func moduleByName(name string) module {
	for _, item := range modules {
		if item.Name == name {
			return item
		}
	}
	return module{}
}

func moduleChoices() string {
	names := make([]string, 0, len(modules))
	for _, item := range modules {
		names = append(names, item.Name)
	}
	return strings.Join(names, ", ")
}
