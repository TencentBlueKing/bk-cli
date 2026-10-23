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
	"os"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
	"github.com/TencentBlueKing/bk-cli/internal/update"
)

// Only these resource/definition reads may run during profile check. Query presets
// (PromQL, spans, alerts, relation batches, etc.) get permission preflight only.
var profileMetadataTools = map[string]bool{
	"get_index_set_fields": true, "get_scene_log_fields": true, "list_time_series_metrics": true,
	"list_time_series_groups": true, "list_apm_services": true, "list_apm_applications": true,
	"get_strategy_detail": true, "get_alarm_strategy": true, "get_dashboard_detail_by_uid": true,
	"get_dashboard_tree_list": true, "get_log_collector": true, "get_log_index_set": true,
	"list_log_collectors": true, "list_index_sets": true, "list_bcs_clusters": true,
}

func v2ProbeTools(b profileBinding) []string {
	if len(b.Tools) > 0 {
		return sortedKeys(b.Tools)
	}
	switch b.Module {
	case "log":
		if b.Options != nil && b.Options.Scene != "" {
			return []string{"get_scene_log_fields"}
		}
		if b.Args["index_set_id"] != nil {
			return []string{"get_index_set_fields"}
		}
		return []string{"list_index_sets"}
	case "metric":
		if b.Args["table_id"] != nil {
			return []string{"list_time_series_metrics"}
		}
		return []string{"list_time_series_groups"}
	case "apm":
		if b.Args["app_name"] != nil {
			return []string{"list_apm_services"}
		}
		return []string{"list_apm_applications"}
	case "alert":
		return []string{"list_alerts"}
	case "alarm":
		return []string{"search_alarm_strategies"}
	case "event":
		return []string{"list_events"}
	case "dashboard":
		if b.Args["dashboard_uid"] != nil {
			return []string{"get_dashboard_detail_by_uid"}
		}
		return []string{"get_dashboard_tree_list"}
	case "relation":
		return []string{"find_relations"}
	case "collect":
		return []string{"list_log_collectors"}
	case "extract":
		return []string{"list_log_extract_topology"}
	default:
		return []string{"list_bcs_clusters"}
	}
}

type bindingCheck struct{ Component, Binding, Tool string }

func v2Checks(p *selectedProfile) ([]bindingCheck, error) {
	result := []bindingCheck{}
	for _, c := range sortedKeys(p.Definition.Components) {
		if p.Component != "" && p.Component != c {
			continue
		}
		for _, b := range sortedKeys(p.Definition.Components[c].Bindings) {
			if p.Binding != "" && p.Binding != b {
				continue
			}
			for _, tool := range v2ProbeTools(p.Definition.Components[c].Bindings[b]) {
				result = append(result, bindingCheck{c, b, tool})
			}
		}
	}
	if len(result) > permissionQueryLimit {
		return nil, monitorapi.InputError(
			"profile_check_too_large",
			"预检超过 20 个工具；用 --component 或 --binding 缩小范围。",
		)
	}
	return result, nil
}

func v2ToolValues(p *selectedProfile, toolName string) (map[string]any, error) {
	values := map[string]any{}
	return values, profileArgs(p, toolsByName[toolName], values, "", nil)
}

func checkV2Binding(rt *monitorapi.Runtime, p *selectedProfile, toolName string) (map[string]any, error) {
	values, err := v2ToolValues(p, toolName)
	if err != nil {
		return nil, err
	}
	result, err := check(rt, p.Profile.BizID, toolName, values)
	if err != nil {
		return nil, err
	}
	result["validation"] = "permission_only"
	metadataTool := toolName
	if p.Route.Module == "metric" && values["table_id"] != nil {
		metadataTool = "list_time_series_metrics"
	}
	if !profileMetadataTools[metadataTool] {
		return result, nil
	}
	copy := *p
	if err := activateBinding(&copy, p.Component, p.Binding, metadataTool); err != nil {
		return nil, err
	}
	metadataArgs, err := v2ToolValues(&copy, metadataTool)
	if err != nil {
		return nil, err
	}
	for key := range bindingResourceArgs {
		if value, ok := values[key]; ok && hasParam(toolsByName[metadataTool], key) {
			metadataArgs[key] = value
		}
	}
	for _, param := range toolsByName[metadataTool].Params {
		if param.Required && metadataArgs[param.Name] == nil {
			return result, nil
		}
	}
	reply, err := monitorapi.Execute(rt, metadataTool, metadataArgs)
	if err != nil {
		return nil, err
	}
	result["validation"], result["metadata_tool"], result["request_id"] = "metadata", metadataTool, reply.RequestID
	return result, nil
}

func v2SourceRuntime(
	cmd *cobra.Command,
	deps systemcmd.BuildDeps,
	source profileSource,
	offline bool,
) (*monitorapi.Runtime, map[string]any, int) {
	row := map[string]any{"context": source.Context, "bk_biz_id": source.BizID}
	if deps.GetContext != nil && deps.GetContext() != "" && deps.GetContext() != source.Context {
		row["status"], row["decision"] = "fail", "profile_context_mismatch"
		return nil, row, output.ExitCodeUserError
	}
	name, cfg, err := config.ResolveContextReadOnly(source.Context)
	if err != nil || cfg == nil {
		row["status"], row["decision"] = "fail", "config_error"
		return nil, row, output.ExitCodeUserError
	}
	if _, err := os.Stat(config.CredentialsPath(name)); err != nil {
		row["status"], row["decision"] = "fail", "auth_required"
		return nil, row, output.ExitCodeUserError
	}
	if offline {
		row["status"], row["authentication"] = "pass", "not_checked"
		return nil, row, 0
	}
	deps.GetContext = func() string { return source.Context }
	rt, err := systemcmd.ResolveRuntime(deps)
	if err != nil {
		failure, code := diagnosticFailure(source.Context, err)
		row["failure"] = failure
		row["status"] = "fail"
		return nil, row, code
	}
	bound := monitorapi.Bind(cmd, rt)
	reply, err := monitorapi.Call(bound, "lookup_tool_schema", map[string]any{"tool_name": "search_spaces"})
	if err != nil {
		failure, code := diagnosticFailure(source.Context, err)
		row["failure"] = failure
		row["status"] = "fail"
		return nil, row, code
	}
	remote, _ := monitorapi.Map(reply.Data)["catalog_version"].(string)
	row["status"], row["authentication"], row["request_id"] = "pass", "verified", reply.RequestID
	row["catalog_local"], row["catalog_remote"] = catalogData.Version, remote
	if remote != catalogData.Version {
		row["status"] = "warn"
	}
	return bound, row, 0
}

func runV2Report(
	cmd *cobra.Command,
	deps systemcmd.BuildDeps,
	p *selectedProfile,
	status, offline, checkUpdate bool,
) error {
	checks, err := v2Checks(p)
	if err != nil {
		return printMonitorFailure(cmd, err)
	}
	originalContext := cmd.Context()
	defer cmd.SetContext(originalContext)
	dry := deps.IsDryRun != nil && deps.IsDryRun()
	if dry {
		plan := []map[string]any{}
		for _, item := range checks {
			b := p.Definition.Components[item.Component].Bindings[item.Binding]
			plan = append(
				plan,
				map[string]any{
					"component": item.Component,
					"binding":   item.Binding,
					"tool":      item.Tool,
					"source":    b.Source,
					"location":  p.FileSpec.Sources[b.Source],
				},
			)
		}
		return writeMonitorEnvelope(
			cmd,
			&output.Envelope{
				OK:     true,
				DryRun: true,
				Data: map[string]any{
					"plan":    plan,
					"network": "not_used",
					"meaning": "仅列出预检计划，未验证身份、资源或权限。",
				},
			},
		)
	}
	rows := []map[string]any{}
	sources := map[string]any{}
	runtimes := map[string]*monitorapi.Runtime{}
	exit := 0
	// Resolve/verify each declared source once per invocation, never persist grants or switch the active context.
	for _, item := range checks {
		b := p.Definition.Components[item.Component].Bindings[item.Binding]
		if _, seen := sources[b.Source]; seen {
			continue
		}
		rt, row, code := v2SourceRuntime(cmd, deps, p.FileSpec.Sources[b.Source], offline)
		sources[b.Source] = row
		runtimes[b.Source] = rt
		if code > exit {
			exit = code
		}
	}
	for _, item := range checks {
		selected, err := cloneBinding(cmd, p, item.Component, item.Binding, item.Tool)
		if err != nil {
			return printMonitorFailure(cmd, err)
		}
		row := map[string]any{
			"component": item.Component,
			"binding":   item.Binding,
			"tool":      item.Tool,
			"source":    selected.Source,
			"context":   selected.Profile.Context,
			"bk_biz_id": selected.Profile.BizID,
		}
		rt := runtimes[selected.Source]
		if rt == nil {
			row["status"], row["validation"] = "skipped", "not_checked"
		} else {
			cmd.SetContext(context.WithValue(cmd.Context(), runtimeContextKey{}, rt.ContextName))
			result, err := checkV2Binding(rt, selected, item.Tool)
			if err == nil {
				row["status"] = "pass"
				row["result"] = result
			} else {
				failure, code := diagnosticFailure(item.Tool, err)
				failure["next"] = profileDiagnosticNext(failure, item.Tool, selected)
				row["status"], row["failure"] = "fail", failure
				if code > exit {
					exit = code
				}
			}
		}
		rows = append(rows, row)
	}
	cmd.SetContext(originalContext)
	data := map[string]any{
		"ok": exit == 0, "sources": sources, "bindings": rows, "offline": offline,
		"meaning": "每个绑定独立检查来源和权限。permission_only 未执行查询，metadata 不证明所有查询表达式或过滤条件有效。",
	}
	if status {
		version := cmd.Root().Annotations["build_version"]
		if version == "" {
			version = "dev"
		}
		data["version"] = version
		data["update"] = map[string]any{"status": "skipped"}
		if checkUpdate && !offline {
			source := &update.GitHubSource{Owner: "TencentBlueKing", Repo: "bk-cli"}
			release, err := source.CheckLatest(cmd.Context())
			data["update"] = releaseCheck(version, release, err)
		}
	}
	return writeDiagnostic(cmd, data, exit)
}
