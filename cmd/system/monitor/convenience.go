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
	"strings"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
)

const (
	sceneFlag = "scene"
	whereFlag = "where"
)

var examples = map[string][]string{
	"search_spaces":   {"bk-cli monitor space search_spaces --space_name 蓝鲸"},
	"list_log_scenes": {"bk-cli monitor log list_log_scenes --bk_biz_id <bk_biz_id>"},
	"get_scene_log_fields": {
		"bk-cli monitor log get_scene_log_fields --bk_biz_id <bk_biz_id> --scene host",
		"bk-cli monitor log get_scene_log_fields --bk_biz_id <bk_biz_id> --scene k8s --where cluster_id=<cluster_id>",
	},
	"search_logs": {
		"bk-cli monitor log search_logs --bk_biz_id <bk_biz_id> --scene host --query_string ERROR --since 15m",
		"bk-cli monitor log search_logs --bk_biz_id <bk_biz_id> --index_set_id <index_set_id>" +
			" --query_string ERROR --since 1h",
	},
	"search_index_set_context": {
		"bk-cli monitor log search_logs --bk_biz_id <bk_biz_id> --index_set_id <id> --limit 1 --since 1h > hit.json",
		"bk-cli monitor log search_index_set_context --bk_biz_id <bk_biz_id> --from hit.json --size 5",
	},
	"list_alerts":    {"bk-cli monitor alert list_alerts --bk_biz_id <bk_biz_id> --status ABNORMAL --since 1h"},
	"get_alert_info": {"bk-cli monitor alert get_alert_info --bk_biz_id <bk_biz_id> --id <alert_id>"},
	"execute_range_query": {
		"bk-cli monitor metric execute_range_query --bk_biz_id <bk_biz_id>" +
			" --promql 'sum(custom:<table:id>:<metric>)' --since 1h",
	},
}

func curatedExample(name string) string {
	lines := examples[name]
	for i := range lines {
		lines[i] = "  " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func addConvenienceFlags(cmd *cobra.Command, item tool) {
	if item.Name == "search_logs" {
		cmd.Flags().StringArray(
			"filter",
			nil,
			"Log row FIELD=VALUE phrase filter; repeatable, AND with query_string; not table routing",
		)
	}
	switch item.Name {
	case "search_logs", "get_scene_log_fields":
		cmd.Flags().String(
			sceneFlag,
			"",
			"Scene from list_log_scenes, such as host or k8s; builds table_id_conditions",
		)
		cmd.Flags().StringArray(
			whereFlag,
			nil,
			"Table route KEY=VALUE, such as cluster_id; use --filter or query_string for row fields like namespace",
		)
	case "search_index_set_context":
		cmd.Flags().String(
			"from",
			"",
			"Saved search_logs output or one log record (@file is optional, - for stdin)",
		)
	}
}

// applyConvenienceFlags only builds documented API arguments; it does not infer resources or permissions.
func applyConvenienceFlags(cmd *cobra.Command, item tool, args map[string]any) error {
	switch item.Name {
	case "search_logs", "get_scene_log_fields":
		p := currentProfile(cmd)
		if p == nil || p.Profile.Log == nil || p.Profile.Log.Scene == "" {
			if err := applyScene(cmd, item, args); err != nil {
				return err
			}
		}
		if item.Name == "search_logs" {
			return applyLogFilters(cmd, args)
		}
		return nil
	case "search_index_set_context":
		return applyContextAnchor(cmd, args)
	}
	return nil
}

func applyScene(cmd *cobra.Command, item tool, args map[string]any) error {
	scene, _ := cmd.Flags().GetString(sceneFlag)
	where, _ := cmd.Flags().GetStringArray(whereFlag)
	if scene == "" {
		if len(where) > 0 {
			return monitorapi.InputError("where_needs_scene", "--where 需要和 --scene 一起使用。")
		}
		if item.Name == "search_logs" && args["index_set_id"] == nil && args["table_id_conditions"] == nil {
			biz := jsonText(args["bk_biz_id"])
			return monitorapi.InputError("needs_resource", "需要选择日志场景或索引集。这不是无权。",
				"bk-cli monitor log list_log_scenes --bk_biz_id "+biz,
				"bk-cli monitor log list_index_sets --bk_biz_id "+biz)
		}
		if item.Name == "search_logs" && args["index_set_id"] != nil && args["target_type"] == nil {
			args["target_type"] = "index_set"
		}
		return nil
	}
	if args["index_set_id"] != nil || args["table_id_conditions"] != nil {
		return monitorapi.InputError(
			"conflicting_target",
			"--scene 不能与 --index_set_id 或 --table_id_conditions 同时使用。",
		)
	}
	conditions := []any{map[string]any{"field_name": "scene", "value": []any{scene}, "op": "eq"}}
	for _, raw := range where {
		key, value, ok := strings.Cut(raw, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" || value == "" {
			return monitorapi.InputError("invalid_where", "--where 使用 KEY=VALUE。")
		}
		conditions = append(conditions, map[string]any{"field_name": key, "value": []any{value}, "op": "eq"})
	}
	args["table_id_conditions"] = []any{conditions}
	if item.Name == "search_logs" {
		args["target_type"] = "scene"
	}
	return nil
}
