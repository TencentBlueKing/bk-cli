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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type options struct {
	input, table, query, since, start, end, field, step, aggregate, groupBy, orderBy, traceID, traceField string
	limit, offset                                                                                         int
	instant                                                                                               bool
	filters                                                                                               []string
}

func decodeObject(r io.Reader) (map[string]any, error) {
	d := json.NewDecoder(r)
	d.UseNumber()
	var obj map[string]any
	if err := d.Decode(&obj); err != nil || obj == nil {
		return nil, fmt.Errorf("input must be a JSON object")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("input must contain exactly one JSON object")
	}
	return obj, nil
}

func timestamp(s string) (int64, error) {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 && len(s) <= 10 {
		return n, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil && t.Unix() > 0 {
		return t.Unix(), nil
	}
	return 0, fmt.Errorf("time must be Unix seconds or RFC3339")
}

func timeRange(o options, now time.Time) (string, string, error) {
	end := now.Unix()
	var err error
	if o.end != "" {
		end, err = timestamp(o.end)
		if err != nil {
			return "", "", err
		}
	}
	var start int64
	if o.start != "" {
		start, err = timestamp(o.start)
	} else {
		var d time.Duration
		d, err = time.ParseDuration(o.since)
		if err == nil && d < time.Second {
			err = fmt.Errorf("--since must be at least 1s")
		}
		start = end - int64(d/time.Second)
	}
	if err != nil {
		return "", "", err
	}
	if start <= 0 || start >= end {
		return "", "", fmt.Errorf("start must be positive and before end")
	}
	return strconv.FormatInt(start, 10), strconv.FormatInt(end, 10), nil
}

func csv(s string) []string {
	out := []string{}
	for v := range strings.SplitSeq(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func buildQuery(command string, o options, stdin io.Reader, now time.Time) (map[string]any, error) {
	if o.input != "" {
		body, err := readInput(o.input, stdin)
		if err != nil {
			return nil, err
		}
		return body, validateQueryList(body)
	}
	if o.table == "" && command != "metrics" {
		return nil, fmt.Errorf("--table is required (or use --input)")
	}
	if o.limit <= 0 || o.offset < 0 {
		return nil, fmt.Errorf("--limit must be positive and --from non-negative")
	}
	start, end, err := timeRange(o, now)
	if err != nil {
		return nil, err
	}
	q := map[string]any{"table_id": o.table, "reference_name": "a"}
	body := map[string]any{"query_list": []any{q}, "start_time": start, "end_time": end, "limit": o.limit}
	filters := append([]string{}, o.filters...)
	if command == "trace" {
		if o.traceID == "" {
			return nil, fmt.Errorf("--id is required for trace (or use --input)")
		}
		if strings.TrimSpace(o.traceField) == "" {
			return nil, fmt.Errorf("--trace-field must not be empty")
		}
		filters = append(filters, o.traceField+"="+o.traceID)
	}
	fields := []any{}
	conditions := []string{}
	for _, filter := range filters {
		name, value, ok := strings.Cut(filter, "=")
		if !ok || strings.TrimSpace(name) == "" || value == "" {
			return nil, fmt.Errorf("--where expects FIELD=VALUE")
		}
		if len(fields) > 0 {
			conditions = append(conditions, "and")
		}
		fields = append(fields, map[string]any{"field_name": name, "op": "eq", "value": []string{value}})
	}
	if len(fields) > 0 {
		q["conditions"] = map[string]any{"field_list": fields, "condition_list": conditions}
	}
	if command == "metrics" {
		if o.field == "" {
			return nil, fmt.Errorf("--field is required for metrics (or use --input)")
		}
		d, err := time.ParseDuration(o.step)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("--step must be a positive duration")
		}
		q["field_name"] = o.field
		if o.aggregate != "" {
			q["function"] = []any{map[string]any{"method": o.aggregate, "dimensions": csv(o.groupBy)}}
		} else if o.groupBy != "" {
			return nil, fmt.Errorf("--group-by requires --aggregate")
		}
		body["metric_merge"] = "a"
		body["step"] = o.step
		body["instant"] = o.instant
	} else {
		if o.query != "" {
			q["query_string"] = o.query
		}
		body["from"] = o.offset
		if o.orderBy != "" {
			body["order_by"] = csv(o.orderBy)
		}
	}
	return body, nil
}

// Apply the environment namespace to every metric query, including JSON input.
// UQ's flat boolean representation cannot safely wrap an arbitrary OR group.
func scopeMetrics(body map[string]any, namespace string) error {
	if namespace == "" {
		return nil
	}
	queries, ok := body["query_list"].([]any)
	if !ok {
		return fmt.Errorf("query_list must be an array")
	}
	for _, item := range queries {
		q, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("query_list entries must be objects")
		}
		conditions := map[string]any{}
		if raw, exists := q["conditions"]; exists {
			var ok bool
			conditions, ok = raw.(map[string]any)
			if !ok {
				return fmt.Errorf("metric conditions must be an object")
			}
		}
		// Normalize generated []string and JSON []any without losing number precision.
		encoded, err := json.Marshal(conditions)
		if err != nil {
			return err
		}
		normalized, err := decodeObject(bytes.NewReader(encoded))
		if err != nil {
			return err
		}
		fields := []any{}
		relations := []any{}
		if v, exists := normalized["field_list"]; exists {
			var ok bool
			fields, ok = v.([]any)
			if !ok {
				return fmt.Errorf("metric field_list must be an array")
			}
		}
		if v, exists := normalized["condition_list"]; exists {
			var ok bool
			relations, ok = v.([]any)
			if !ok {
				return fmt.Errorf("metric condition_list must be an array")
			}
		}
		expected := max(len(fields)-1, 0)
		if len(relations) != expected {
			return fmt.Errorf("invalid metric condition_list length")
		}
		for _, r := range relations {
			if r != "and" {
				return fmt.Errorf("environment namespace filtering requires AND-only metric conditions")
			}
		}
		found := false
		for _, raw := range fields {
			f, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("metric condition must be an object")
			}
			if f["field_name"] == "namespace" {
				values, ok := f["value"].([]any)
				if !ok || len(values) != 1 || values[0] != namespace || f["op"] != "eq" {
					return fmt.Errorf(
						"namespace condition conflicts with environment; use --namespace to select another namespace",
					)
				}
				found = true
			}
		}
		if !found {
			if len(fields) > 0 {
				relations = append(relations, "and")
			}
			fields = append(
				fields,
				map[string]any{"field_name": "namespace", "op": "eq", "value": []string{namespace}},
			)
		}
		normalized["field_list"] = fields
		normalized["condition_list"] = relations
		q["conditions"] = normalized
	}
	return nil
}

func readInput(path string, stdin io.Reader) (map[string]any, error) {
	if path == "-" {
		return decodeObject(stdin)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodeObject(bytes.NewReader(raw))
}

func validateQueryList(body map[string]any) error {
	queries, ok := body["query_list"].([]any)
	if !ok || len(queries) == 0 {
		return fmt.Errorf("input requires a non-empty query_list")
	}
	for _, query := range queries {
		if _, ok := query.(map[string]any); !ok {
			return fmt.Errorf("query_list entries must be objects")
		}
	}
	for _, key := range []string{"start_time", "end_time"} {
		if value, ok := body[key].(string); !ok || value == "" {
			return fmt.Errorf("input requires string %s", key)
		}
	}
	return nil
}
