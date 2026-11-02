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

// Package monitorapi calls individual Monitor HTTP APIs through the shared request executor.
// The server remains the source of truth for tool contracts, resource ownership and permissions.
package monitorapi

import (
	"fmt"
	"maps"
	"strings"
)

// alertRequestArgs derives native search scope after Profiles and flags resolve.
func alertRequestArgs(tool string, args map[string]any) (map[string]any, error) {
	if tool != "list_alerts" && tool != "get_alert_top_n" {
		return args, nil
	}
	biz := strings.TrimSpace(fmt.Sprint(args["bk_biz_id"]))
	if args["bk_biz_id"] == nil || biz == "" {
		return nil, InputError("missing_arguments", "告警查询需要明确的 bk_biz_id。")
	}
	if provided, exists := args["bk_biz_ids"]; exists {
		var ids []string
		switch values := provided.(type) {
		case []any:
			for _, value := range values {
				ids = append(ids, strings.TrimSpace(fmt.Sprint(value)))
			}
		case []string:
			ids = values
		}
		if len(ids) != 1 || ids[0] != biz {
			return nil, InputError("invalid_arguments", "bk_biz_ids 必须与最终 bk_biz_id 一致且只含一个业务；请移除冲突值。")
		}
	}
	result := maps.Clone(args)
	result["bk_biz_ids"] = []string{biz}
	return result, nil
}

// alertListScopeValid checks query scope; it never grants permission or switches
// business based on a response. Unexpected rows must not reach final output.
func alertListScopeValid(data, biz any) (listValid, scopeValid bool) {
	rows, ok := Map(data)["alerts"].([]any)
	if !ok {
		return false, false
	}
	expected := strings.TrimSpace(fmt.Sprint(biz))
	for _, row := range rows {
		value := Map(row)["bk_biz_id"]
		if value == nil || strings.TrimSpace(fmt.Sprint(value)) != expected {
			return true, false
		}
	}
	return true, true
}
