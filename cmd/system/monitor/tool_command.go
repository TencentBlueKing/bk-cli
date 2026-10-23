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
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

var now = time.Now

const sinceFlag = "since"

func newToolCmd(item tool, deps systemcmd.BuildDeps) *cobra.Command {
	var body, since string
	hasWindow := hasParam(item, "start_time") && hasParam(item, "end_time")
	cmd := &cobra.Command{
		Use:     item.Name,
		Short:   item.Short,
		Long:    longHelp(item, hasWindow),
		Example: example(item, hasWindow),
		Args:    cobra.NoArgs,
		RunE: runMonitor(deps, func(cmd *cobra.Command, rt *monitorapi.Runtime) error {
			args, err := toolArgs(cmd, item, body, since, hasWindow)
			if err != nil {
				return err
			}
			reply, err := monitorapi.Execute(rt, item.Name, args)
			if err != nil {
				return preservePermissionTarget(
					err,
					rt.ContextName,
					item.Name,
					jsonText(args["bk_biz_id"]),
					args,
				)
			}
			return printReply(cmd, reply, nil)
		}),
	}
	addProfileFlags(cmd)
	for _, p := range item.Params {
		addParamFlag(cmd, p)
	}
	addConvenienceFlags(cmd, item)
	if hasTimeInput(item) {
		cmd.Flags().String(
			"timezone",
			defaultTimeZone,
			"IANA zone for wall-clock dates and today/yesterday; RFC3339 offsets and Unix timestamps are absolute",
		)
	}
	if hasWindow {
		cmd.Flags().StringVar(
			&since,
			sinceFlag,
			"",
			"Recent time window ending now, such as 15m or 1h (max 24h); fills start_time/end_time",
		)
	}
	cmd.Flags().StringVar(
		&body,
		syslib.ActionBodyFlagName,
		"",
		"Full tool_args JSON, @file, or - for stdin; named flags override matching keys",
	)
	return cmd
}

func addParamFlag(cmd *cobra.Command, p param) {
	usage := p.Description
	if p.Required {
		usage = "[Required] " + usage
	}
	if len(p.Enum) > 0 {
		usage += "; values: " + strings.Join(p.Enum, ", ")
	}
	switch {
	case p.Type == "array" && isScalar(p.Items):
		usage += "; repeat or comma-separate"
		cmd.Flags().StringArray(p.Name, nil, usage)
	case p.Type == "array" || p.Type == "object":
		usage += "; JSON"
		cmd.Flags().String(p.Name, "", usage)
	case p.Type == "boolean":
		cmd.Flags().String(p.Name, "", usage+"; true or false")
	case p.TimeUnit != "":
		cmd.Flags().String(
			p.Name,
			"",
			usage+"; datetime/RFC3339/today/yesterday/now or Unix "+timeUnitName(
				p.TimeUnit,
			)+" (optional s/ms suffix)",
		)
	default:
		cmd.Flags().String(p.Name, "", usage)
	}
}

// toolArgs merges --body with named flags, converts only by the reviewed schema type, and leaves
// all ownership and permission checks to the server.
func toolArgs(cmd *cobra.Command, item tool, body, since string, hasWindow bool) (map[string]any, error) {
	args := map[string]any{}
	if body != "" {
		raw, err := readBody(cmd, body)
		if err != nil {
			return nil, err
		}
		if err := jsonObject(raw, &args); err != nil {
			return nil, err
		}
	}
	for _, p := range item.Params {
		flag := cmd.Flags().Lookup(p.Name)
		if flag == nil || !flag.Changed {
			continue
		}
		value, err := convertFlag(cmd, p)
		if err != nil {
			return nil, err
		}
		args[p.Name] = value
	}
	scene, _ := cmd.Flags().GetString(sceneFlag)
	where, _ := cmd.Flags().GetStringArray(whereFlag)
	if err := profileArgs(currentProfile(cmd), item, args, scene, where); err != nil {
		return nil, err
	}
	if err := applyConvenienceFlags(cmd, item, args); err != nil {
		return nil, err
	}
	if err := applyTimeInputs(cmd, item, args, since); err != nil {
		return nil, err
	}
	var missing []string
	for _, p := range item.Params {
		if p.Required && args[p.Name] == nil {
			missing = append(missing, "--"+p.Name)
		}
	}
	if len(missing) > 0 {
		next := monitorapi.NextCommands(item.Name, fmt.Sprint(args["bk_biz_id"]))
		hint := "缺少必填参数：" + strings.Join(missing, ", ")
		if hasWindow && (args["start_time"] == nil || args["end_time"] == nil) {
			hint += "；时间可用 --since 15m"
		}
		return nil, monitorapi.InputError("missing_arguments", hint+"。", next...)
	}
	recordProfileArgs(currentProfile(cmd), args)
	return args, nil
}

func readBody(cmd *cobra.Command, body string) ([]byte, error) {
	var data []byte
	var err error
	switch {
	case body == "-":
		data, err = readAllLimited(cmd.InOrStdin())
	case strings.HasPrefix(body, "@"):
		data, err = readFileLimited(strings.TrimPrefix(body, "@"))
	default:
		data = []byte(body)
	}
	if err != nil || len(data) == 0 || len(data) > maxInputBytes {
		return nil, monitorapi.InputError("invalid_body", "无法读取 --body，或内容为空、超过 64 KiB。")
	}
	return data, nil
}

func convertFlag(cmd *cobra.Command, p param) (any, error) {
	if p.TimeUnit != "" {
		raw, _ := cmd.Flags().GetString(p.Name)
		return raw, nil
	}
	if p.Type == "array" && isScalar(p.Items) {
		values, _ := cmd.Flags().GetStringArray(p.Name)
		var items []any
		for _, raw := range values {
			for part := range strings.SplitSeq(raw, ",") {
				if part = strings.TrimSpace(part); part != "" {
					value, err := scalar(p.Name, p.Items, part)
					if err != nil {
						return nil, err
					}
					items = append(items, value)
				}
			}
		}
		return items, nil
	}
	raw, _ := cmd.Flags().GetString(p.Name)
	if p.Type == "array" || p.Type == "object" {
		var value any
		if err := decodeJSON([]byte(raw), &value); err != nil {
			return nil, monitorapi.InputError("invalid_json_flag", "--"+p.Name+" 必须是 JSON。")
		}
		if (p.Type == "array" && monitorapi.List(value) == nil) ||
			(p.Type == "object" && monitorapi.Map(value) == nil) {
			return nil, monitorapi.InputError("invalid_json_flag", "--"+p.Name+" 的 JSON 类型必须是 "+p.Type+"。")
		}
		return value, nil
	}
	return scalar(p.Name, p.Type, raw)
}

func scalar(name, kind, raw string) (any, error) {
	switch kind {
	case "integer":
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, monitorapi.InputError("invalid_integer", "--"+name+" 必须是整数。")
		}
		return value, nil
	case "number":
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, monitorapi.InputError("invalid_number", "--"+name+" 必须是数字。")
		}
		return value, nil
	case "boolean":
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, monitorapi.InputError("invalid_boolean", "--"+name+" 只能是 true 或 false。")
		}
		return value, nil
	default:
		return raw, nil
	}
}

func longHelp(item tool, hasWindow bool) string {
	lines := []string{
		item.Short + ".",
		"",
		"Monitor API tool: " + item.Name + " (" + item.Category + ").",
		"Each run is checked again by bk-monitor for identity, resource ownership and permission.",
		"Use --profile NAME for SRE-provided resource bindings; inspect with monitor profile show --profile NAME.",
	}
	if len(item.Prerequisites) > 0 {
		lines = append(
			lines,
			"If an identifier is missing, get it from: "+strings.Join(item.Prerequisites, ", ")+".",
		)
	}
	if hasWindow {
		lines = append(
			lines,
			"Use --since for a recent window OR --start_time/--end_time for explicit endpoints (including body).",
		)
	}
	if hasTimeInput(item) {
		lines = append(
			lines,
			"Time input: YYYY-MM-DD HH:MM[:SS], RFC3339, today/yesterday/now (今天/昨天/现在), or integer Unix timestamps.",
			"Wall-clock default: Asia/Shanghai; override with --timezone. Explicit RFC3339 offsets take precedence.",
			"Unsuffixed timestamps use "+timeUnitName(
				timeUnit(item),
			)+"; s/ms suffixes explicitly convert units. No digit-count guessing.",
			"Only catalog top-level time fields are converted; query_list and --from anchor timestamps remain native.",
		)
	}
	lines = append(lines, "Use --body for the full tool_args JSON; named flags override matching keys.", "")
	lines = append(lines, permissionLines(item)...)
	return strings.Join(lines, "\n")
}

func example(item tool, hasWindow bool) string {
	if text := curatedExample(item.Name); text != "" {
		return text
	}
	parts := []string{"bk-cli monitor", item.Module, item.Name}
	for _, p := range item.Params {
		if !p.Required || (hasWindow && (p.Name == "start_time" || p.Name == "end_time")) {
			continue
		}
		parts = append(parts, "--"+p.Name, placeholder(p))
	}
	if hasWindow {
		parts = append(parts, "--since", "15m")
	}
	return "  " + strings.Join(parts, " ")
}

func placeholder(p param) string {
	switch {
	case p.Name == "bk_biz_id":
		return "<bk_biz_id>"
	case p.Type == "array" && isScalar(p.Items):
		if len(p.Enum) > 0 {
			return p.Enum[0]
		}
		return "<value>"
	case p.Type == "array":
		return "'[...]'"
	case p.Type == "object":
		return "'{...}'"
	case len(p.Enum) > 0:
		return p.Enum[0]
	default:
		return "<" + p.Name + ">"
	}
}

func hasParam(item tool, name string) bool {
	for _, p := range item.Params {
		if p.Name == name {
			return true
		}
	}
	return false
}

func timeUnit(item tool) string {
	for _, p := range item.Params {
		if p.Name == "start_time" && p.TimeUnit != "" {
			return p.TimeUnit
		}
	}
	return "s"
}

func timeUnitName(unit string) string {
	if unit == "ms" {
		return "milliseconds"
	}
	return "seconds"
}

func isScalar(kind string) bool {
	return kind == "string" || kind == "integer" || kind == "number" || kind == "boolean"
}
