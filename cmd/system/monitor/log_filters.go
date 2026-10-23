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
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
)

var logFilterFieldPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

func validateLogFilters(filters map[string]string) error {
	for key, value := range filters {
		if !logFilterFieldPattern.MatchString(key) || strings.TrimSpace(value) == "" ||
			strings.ContainsFunc(value, unicode.IsControl) || strings.ContainsAny(value, "\u2028\u2029") {
			return monitorapi.InputError("invalid_log_filter", "日志过滤需要合法字段名和非空单行值；复杂表达式用 --query_string。")
		}
	}
	return nil
}

// Row filters are distinct from table routing. Compile literal field phrases into
// the existing query_string API; never invent a new backend parameter or switch sources.
func applyLogFilters(cmd *cobra.Command, args map[string]any) error {
	filters := map[string]string{}
	selection := currentProfile(cmd)
	if selection != nil && selection.Profile.Log != nil {
		maps.Copy(filters, selection.Profile.Log.Filters)
	}
	flags, _ := cmd.Flags().GetStringArray("filter")
	for _, raw := range flags {
		key, value, ok := strings.Cut(raw, "=")
		if !ok {
			return monitorapi.InputError("invalid_log_filter", "--filter 使用 FIELD=VALUE，可重复。")
		}
		filters[strings.TrimSpace(key)] = value
	}
	if len(filters) == 0 {
		return nil
	}
	if err := validateLogFilters(filters); err != nil {
		return err
	}
	keys := make([]string, 0, len(filters))
	for key := range filters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	terms := make([]string, 0, len(keys))
	for _, key := range keys {
		// Escape Lucene phrase delimiters, not HTML or JSON Unicode sequences.
		value := strings.ReplaceAll(filters[key], `\`, `\\`)
		value = strings.ReplaceAll(value, `"`, `\"`)
		terms = append(terms, key+`:"`+value+`"`)
	}
	expression := strings.Join(terms, " AND ")
	query := ""
	if value, exists := args["query_string"]; exists {
		var ok bool
		query, ok = value.(string)
		if !ok {
			return monitorapi.InputError("invalid_query_string", "query_string 必须是字符串。")
		}
	}
	if strings.TrimSpace(query) != "" && query != "*" {
		expression = "(" + query + ") AND (" + expression + ")"
	}
	args["query_string"] = expression
	if selection != nil {
		selection.RowFilters = filters
		if selection.Profile.Log != nil && len(selection.Profile.Log.Filters) > 0 &&
			!slices.Contains(selection.Applied, "log.filters") {
			selection.Applied = append(selection.Applied, "log.filters")
		}
	}
	return nil
}
