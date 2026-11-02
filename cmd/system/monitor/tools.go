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
	"fmt"
	"maps"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newToolsSpec(byName map[string]tool) systemcmd.SystemSpec {
	return systemcmd.SystemSpec{
		Name:        "tools",
		Description: "Search tools or read one server contract",
		RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
			parent.AddCommand(newSearchToolsCmd(deps), newSchemaCmd(deps, byName))
			return nil
		},
	}
}

func newSearchToolsCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var biz, category, capability string
	var availableOnly bool
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search Monitor API tools for one business",
		Long: `Search tools for one business.

permission_state=requires_resource means a concrete resource is still needed.
It is neither denial nor grant. Use monitor permission check after choosing the resource.`,
		Example: "  bk-cli monitor tools search --bk_biz_id <bk_biz_id> --category log\n" +
			"  bk-cli monitor tools search --bk_biz_id <bk_biz_id> --capability query",
		Args: cobra.NoArgs,
		RunE: runMonitor(deps, func(cmd *cobra.Command, rt *monitorapi.Runtime) error {
			if err := monitorapi.RequireBiz(biz); err != nil {
				return err
			}
			body := map[string]any{"bk_biz_id": biz, "available_only": availableOnly, "page_size": 100}
			if category != "" {
				body["category"] = categoryFor(category)
			}
			if capability != "" {
				body["capability"] = capability
			}
			reply, err := monitorapi.Call(rt, "lookup_tool", body)
			if err != nil || reply.IsDryRun() {
				return printReply(cmd, reply, err)
			}
			data := monitorapi.Map(reply.Data)
			for _, item := range monitorapi.List(data["tools"]) {
				card := monitorapi.Map(item)
				if command := commandForTool(card); command != "" {
					card["command"] = command
				}
			}
			return writeMonitorEnvelope(cmd, reply.Envelope(data))
		}),
	}
	addProfileFlags(cmd)
	cmd.Flags().StringVar(&biz, "bk_biz_id", "", "Business ID")
	cmd.Flags().StringVar(&category, "category", "", "CLI module or API category, such as log or alert")
	cmd.Flags().StringVar(
		&capability,
		"capability",
		"",
		"Capability: discovery, query, analysis, detail, relation, mutation, export",
	)
	cmd.Flags().BoolVar(
		&availableOnly,
		"available_only",
		false,
		"Hide tools explicitly denied; keep requires_resource",
	)
	return cmd
}

func newSchemaCmd(deps systemcmd.BuildDeps, byName map[string]tool) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:     "schema",
		Short:   "Show the current server contract for one tool",
		Example: "  bk-cli monitor tools schema --tool search_logs",
		Args:    cobra.NoArgs,
		RunE: runMonitor(deps, func(cmd *cobra.Command, rt *monitorapi.Runtime) error {
			if _, ok := byName[name]; !ok {
				return monitorapi.InputError(
					"invalid_tool",
					"未知的只读工具；先运行 bk-cli monitor tools search --bk_biz_id <bk_biz_id>。",
				)
			}
			reply, err := monitorapi.Call(rt, "lookup_tool_schema", map[string]any{"tool_name": name})
			return printReply(cmd, reply, err)
		}),
	}
	addProfileFlags(cmd)
	cmd.Flags().StringVar(&name, "tool", "", "Monitor API tool name")
	return cmd
}

// check runs the Unified Tool Search decision chain without executing the data tool.
func check(rt *monitorapi.Runtime, biz, name string, values map[string]any) (map[string]any, error) {
	var steps []map[string]any
	catalog, err := monitorapi.Call(
		rt,
		"lookup_tool",
		map[string]any{"tool_name": name, "bk_biz_id": biz, "available_only": false},
	)
	if err != nil {
		return nil, err
	}
	if catalog.IsDryRun() {
		return nil, monitorapi.InputError(
			"dry_run_not_supported",
			"permission check/apply 需要真实服务端判定，不支持 --dry-run。",
		)
	}
	cards := monitorapi.List(monitorapi.Map(catalog.Data)["tools"])
	var card map[string]any
	if len(cards) > 0 {
		card = monitorapi.Map(cards[0])
	}
	if card["name"] != name {
		return nil, &monitorapi.Decision{
			Code:      "catalog_mismatch",
			Message:   "工具目录没有返回该精确工具。",
			ExitCode:  output.ExitCodeSystemError,
			RequestID: catalog.RequestID,
		}
	}
	if card["risk"] != "query" || card["requires_confirmation"] != false {
		return nil, &monitorapi.Decision{
			Code:      "readonly_contract_changed",
			Message:   "服务端工具不再是只读契约，已停止。",
			ExitCode:  output.ExitCodeSystemError,
			RequestID: catalog.RequestID,
		}
	}
	steps = append(steps, map[string]any{
		"step": "tool_search", "request_id": catalog.RequestID, "permission_state": card["permission_state"],
		"required_context": card["required_context"], "prerequisites": card["prerequisites"],
	})
	schema, err := monitorapi.Call(rt, "lookup_tool_schema", map[string]any{"tool_name": name})
	if err != nil {
		return nil, err
	}
	contract := monitorapi.Map(schema.Data)
	context, unused := monitorapi.ResourceContext(contract, values)
	permission := monitorapi.Map(contract["permission"])
	steps = append(steps, map[string]any{
		"step":            "contract",
		"request_id":      schema.RequestID,
		"catalog_version": contract["catalog_version"],
		"permission_mode": permission["mode"],
		"fallback_on":     permission["fallback_on"],
		"ignored_fields":  unused,
	})
	body := map[string]any{"tool_name": name, "bk_biz_id": biz, "include_apply_guide": true}
	if len(context) > 0 {
		body["resource_context"] = context
	}
	decision, step, err := permissionDecision(rt, name, body, context, permission)
	if err != nil {
		return nil, err
	}
	steps = append(steps, step)
	if decision != nil {
		if monitorapi.IsResourceDecision(decision.Code) {
			decision.Next = nextCommands(name, biz)
		}
		decision.Steps = steps
		return nil, preservePermissionTarget(decision, rt.ContextName, name, biz, context)
	}
	return map[string]any{
		"decision": "ready", "tool": name, "bk_biz_id": biz, "steps": steps,
		"next":    commandName(name) + " --help",
		"meaning": "服务端预检通过。真正执行时仍会重新鉴权；结果不缓存、不当凭据。",
	}, nil
}

func permissionDecision(
	rt *monitorapi.Runtime,
	name string,
	body, context, permission map[string]any,
) (*monitorapi.Decision, map[string]any, error) {
	reply, err := monitorapi.Call(rt, "lookup_permissions", body)
	if err != nil {
		judgment, ok := monitorapi.IsJudgment(err)
		if !ok {
			return nil, nil, err
		}
		return judgment, map[string]any{
			"step":             "permission",
			"request_id":       judgment.RequestID,
			"resource_context": context,
			"decision":         judgment.Code,
		}, nil
	}
	data := monitorapi.Map(reply.Data)
	var state map[string]any
	matches := 0
	for _, item := range monitorapi.List(data["scopes"]) {
		if scope := monitorapi.Map(item); scope["tool_name"] == name {
			state = scope
			matches++
		}
	}
	state = normalizeLegacyPermissionState(state, permission, body["bk_biz_id"])
	authorized, hasAuthorization := data["authorized"].(bool)
	granted := state["state"] == "granted" || state["state"] == "exempt"
	if matches != 1 || len(monitorapi.List(data["scopes"])) != 1 || !hasAuthorization || authorized != granted ||
		(authorized && len(monitorapi.List(data["missing_permissions"])) > 0) ||
		(state["authorized"] != nil && state["authorized"] != authorized) {
		return nil, nil, &monitorapi.Decision{
			Code: "permission_response_invalid", Message: "权限响应没有唯一匹配当前工具，或授权结论互相矛盾；已停止。",
			ExitCode: output.ExitCodeSystemError, RequestID: reply.RequestID,
		}
	}
	step := map[string]any{
		"step": "permission", "request_id": reply.RequestID, "resource_context": context,
		"state": state["state"], "authorization_source": state["authorization_source"],
	}
	if authorized {
		return nil, step, nil
	}
	decision := monitorapi.StateDecision(state)
	if decision == nil {
		decision = monitorapi.StateDecision(map[string]any{})
	}
	decision.RequestID = reply.RequestID
	return decision, step, nil
}

// Older space-scoped APIs return an explicit authorization boolean without a state.
// Accept that contract only when the server schema, action and business all match.
// This normalizes a server decision; it never derives permission from the catalog.
func normalizeLegacyPermissionState(scope, permission map[string]any, biz any) map[string]any {
	if _, exists := scope["state"]; exists {
		return scope
	}
	action, hasAction := permission["action_id"].(string)
	authorized, hasAuthorization := scope["authorized"].(bool)
	_, hasMode := permission["mode"]
	resource := monitorapi.Map(scope["resource"])
	if hasMode || !hasAction || action == "" || !hasAuthorization ||
		permission["resource_type"] != "space" || permission["resource_arg"] != "bk_biz_id" ||
		scope["action_id"] != action || biz == nil || resource["bk_biz_id"] == nil ||
		fmt.Sprint(resource["bk_biz_id"]) != fmt.Sprint(biz) {
		return scope
	}
	state := maps.Clone(scope)
	state["state"] = "missing"
	if authorized {
		state["state"] = "granted"
	}
	state["authorization_source"] = "legacy_mcp"
	return state
}

func categoryFor(value string) string {
	for _, item := range modules {
		if item.Name == value || item.Category == value {
			return item.Category
		}
	}
	return value
}

func commandForTool(card map[string]any) string {
	name, _ := card["name"].(string)
	if card["risk"] != "query" || card["requires_confirmation"] != false {
		return ""
	}
	return commandName(name)
}
