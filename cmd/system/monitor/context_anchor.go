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
	"strconv"
	"strings"

	json "github.com/goccy/go-json"
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
)

var anchorFields = []string{"dtEventTimeStamp", "serverIp", "gseIndex", "iterationIndex", "path"}

// applyContextAnchor copies exact positioning fields from a saved search result. It never invents values.
func applyContextAnchor(cmd *cobra.Command, args map[string]any) error {
	source, _ := cmd.Flags().GetString("from")
	if source == "" {
		return nil
	}
	var raw []byte
	var err error
	if source == "-" {
		raw, err = readAllLimited(cmd.InOrStdin())
	} else {
		raw, err = readFileLimited(strings.TrimPrefix(source, "@"))
	}
	if err != nil {
		return monitorapi.InputError("invalid_from", "无法读取 --from，或内容超过 64 KiB。")
	}
	record, sourceIndex, provenance, err := firstRecord(raw)
	if err != nil {
		return err
	}
	if err := validateAnchorScope(cmd, args, provenance); err != nil {
		return err
	}
	index, ok, err := sourceIndexID(record["__index_set_id__"], sourceIndex,
		monitorapi.Map(provenance["resolved_parameters"])["index_set_id"])
	if err != nil {
		return err
	}
	if args["index_set_id"] == nil {
		if !ok || index <= 0 {
			return monitorapi.InputError("needs_index_set", "记录里没有索引集 ID；请传 --index_set_id。")
		}
		args["index_set_id"] = index
	} else if ok && strconv.FormatInt(index, 10) != strings.TrimSpace(jsonText(args["index_set_id"])) {
		return monitorapi.InputError("anchor_index_mismatch", "定位记录属于另一个索引集；不改写定位来源。")
	}
	for _, name := range anchorFields {
		if args[name] != nil {
			continue
		}
		value, present, err := anchorValue(record[name])
		if err != nil {
			return monitorapi.InputError("invalid_anchor", name+" 必须是原始字符串或精确整数。")
		}
		if present {
			args[name] = value
		}
	}
	if args["zero"] == nil {
		args["zero"] = true
	}
	if args["begin"] == nil {
		args["begin"] = "0"
	}
	if args["size"] == nil {
		args["size"] = "5"
	}
	return nil
}

func firstRecord(raw []byte) (map[string]any, any, map[string]any, error) {
	var value any
	if err := decodeJSON(raw, &value); err != nil {
		return nil, nil, nil, monitorapi.InputError("invalid_from", "--from 必须是单个完整 JSON。")
	}
	record := monitorapi.Map(value)
	if record["ok"] == false || record["result"] == false {
		return nil, nil, nil, monitorapi.InputError("invalid_from", "失败响应不能作为日志定位来源。")
	}
	if record["__demo_mock__"] == true {
		return nil, nil, nil, monitorapi.InputError("mock_record", "模拟记录不能发往真实服务。")
	}
	provenance := monitorapi.Map(record["profile"])
	if record["profile"] != nil && provenance == nil {
		return nil, nil, nil, monitorapi.InputError("invalid_from", "定位来源的 Profile 信息不合法。")
	}
	var sourceIndex any
	if data := monitorapi.Map(record["data"]); data != nil {
		sourceIndex = data["index_set_id"]
		if rows := monitorapi.List(data["list"]); len(rows) > 0 {
			record = monitorapi.Map(rows[0])
		}
	}
	if record == nil || record["dtEventTimeStamp"] == nil {
		return nil, nil, nil, monitorapi.InputError("invalid_from", "--from 需要 search_logs 的输出，或一条日志记录。")
	}
	if record["__demo_mock__"] == true {
		return nil, nil, nil, monitorapi.InputError("mock_record", "模拟记录不能发往真实服务。")
	}
	return record, sourceIndex, provenance, nil
}

func validateAnchorScope(cmd *cobra.Command, args, provenance map[string]any) error {
	if provenance == nil {
		return nil
	} // Raw records have no verifiable local provenance; server still authorizes.
	sourceContext, _ := provenance["context"].(string)
	if sourceContext == "" || provenance["bk_biz_id"] == nil {
		return monitorapi.InputError("invalid_from", "定位来源缺少 Profile 的 context 或业务信息。")
	}
	current, _ := cmd.Context().Value(runtimeContextKey{}).(string)
	if sourceContext != current {
		return monitorapi.InputError("anchor_context_mismatch", "定位记录来自另一个 context；不跨环境复用日志定位。")
	}
	if jsonText(provenance["bk_biz_id"]) != jsonText(args["bk_biz_id"]) {
		return monitorapi.InputError("anchor_business_mismatch", "定位记录来自另一个业务；不改写定位来源。")
	}
	return nil
}

func sourceIndexID(values ...any) (int64, bool, error) {
	var index int64
	found := false
	for _, value := range values {
		if value == nil {
			continue
		}
		candidate, ok := exactInteger(value)
		if !ok || candidate <= 0 {
			return 0, false, monitorapi.InputError("invalid_anchor_index", "定位来源中的索引集 ID 必须是正整数。")
		}
		if found && index != candidate {
			return 0, false, monitorapi.InputError("anchor_index_mismatch", "定位记录与来源元数据的索引集 ID 不一致。")
		}
		index, found = candidate, true
	}
	return index, found, nil
}

func exactInteger(value any) (int64, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseInt(typed.String(), 10, 64)
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(typed, 10, 64)
		return parsed, err == nil
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	}
	return 0, false
}

func anchorValue(value any) (string, bool, error) {
	switch typed := value.(type) {
	case nil:
		return "", false, nil
	case string:
		return typed, typed != "", nil
	case json.Number:
		if _, err := strconv.ParseInt(typed.String(), 10, 64); err != nil {
			return "", false, err
		}
		return typed.String(), true, nil
	}
	return "", false, strconv.ErrSyntax
}

func jsonText(value any) string {
	switch typed := value.(type) {
	case int64:
		return strconv.FormatInt(typed, 10)
	case string:
		return typed
	default:
		raw, _ := json.Marshal(typed)
		return string(raw)
	}
}
