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
	"time"
	_ "time/tzdata" // Keep IANA zones available in the standalone binary.

	json "github.com/goccy/go-json"
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
)

const defaultTimeZone = "Asia/Shanghai"

var wallTimeLayouts = []string{
	"2006-01-02 15:04:05.999999999", "2006-01-02 15:04",
	"2006-01-02T15:04:05.999999999", "2006-01-02T15:04", "2006-01-02",
}

func hasTimeInput(item tool) bool {
	for _, p := range item.Params {
		if p.TimeUnit != "" {
			return true
		}
	}
	return false
}

// applyTimeInputs normalizes only catalog-declared top-level time parameters.
// Nested query_list times and exact --from anchor fields are not interpreted here.
func applyTimeInputs(cmd *cobra.Command, item tool, args map[string]any, since string) error {
	if !hasTimeInput(item) {
		return nil
	}
	zone, _ := cmd.Flags().GetString("timezone")
	if zone == "" || zone == "Local" {
		return monitorapi.InputError(
			"invalid_timezone",
			"--timezone 使用明确的 IANA 时区（如 Asia/Shanghai）或 UTC，不使用 Local。",
		)
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return monitorapi.InputError("invalid_timezone", "无法识别 --timezone；请使用 Asia/Shanghai、UTC 等 IANA 时区。")
	}
	reference := now() // One clock sample for both endpoints, including calendar keywords.
	parsed := map[string]time.Time{}
	if since != "" || cmd.Flags().Changed(sinceFlag) {
		if _, exists := args["start_time"]; exists {
			return timeConflict()
		}
		if _, exists := args["end_time"]; exists {
			return timeConflict()
		}
		start, end, err := relativeWindow(item, since, reference)
		if err != nil {
			return err
		}
		parsed["start_time"], parsed["end_time"] = start, end
	} else {
		for _, p := range item.Params {
			if p.TimeUnit == "" || args[p.Name] == nil {
				continue
			}
			value, err := parseTimeValue(args[p.Name], p.TimeUnit, location, reference)
			if err != nil {
				return err
			}
			parsed[p.Name] = value
		}
	}
	if start, ok := parsed["start_time"]; ok {
		if end, exists := parsed["end_time"]; exists {
			if !start.Before(end) {
				return monitorapi.InputError(
					"invalid_time_range",
					"start_time 必须早于 end_time；不会交换或扩展时间边界。",
				)
			}
			if item.Name == "search_logs" && end.Sub(start) > 24*time.Hour {
				return monitorapi.InputError(
					"invalid_time_range",
					"日志检索单次窗口不能超过 24h；请显式拆分，CLI 不截断或自动翻页。",
				)
			}
		}
	}
	for _, p := range item.Params {
		if value, exists := parsed[p.Name]; exists {
			normalized, err := encodedTime(p, value)
			if err != nil {
				return err
			}
			args[p.Name] = normalized
		}
	}
	return nil
}

func timeConflict() error {
	return monitorapi.InputError("conflicting_time", "--since 不能与 start_time/end_time 同时使用（命名参数和 --body 都算）。")
}

func timeResolution(unit string) time.Duration {
	if unit == "ms" {
		return time.Millisecond
	}
	return time.Second
}

func relativeWindow(item tool, since string, reference time.Time) (time.Time, time.Time, error) {
	duration, err := time.ParseDuration(since)
	if err != nil || duration <= 0 || duration > 24*time.Hour || duration%timeResolution(timeUnit(item)) != 0 {
		return time.Time{}, time.Time{}, monitorapi.InputError(
			"invalid_since",
			"--since 使用 15m、1h、1h30m 等正时长，不超过 24h，且须满足工具的整秒/整毫秒精度。",
		)
	}
	end := reference.Truncate(timeResolution(timeUnit(item)))
	return end.Add(-duration), end, nil
}

func parseTimeValue(value any, unit string, location *time.Location, reference time.Time) (time.Time, error) {
	var raw string
	switch v := value.(type) {
	case string:
		raw = strings.TrimSpace(v)
	case json.Number:
		raw = v.String()
	case int64:
		raw = strconv.FormatInt(v, 10)
	case int:
		raw = strconv.Itoa(v)
	default:
		return time.Time{}, invalidTime()
	}
	lower := strings.ToLower(raw)
	if lower == "now" || lower == "现在" {
		return reference.Truncate(timeResolution(unit)), nil
	}
	if replaced, ok := calendarTime(lower, location, reference); ok {
		raw = replaced
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return checkedTime(t, unit)
	}
	inputUnit := unit
	numeric := raw
	switch {
	case strings.HasSuffix(raw, "ms"):
		inputUnit, numeric = "ms", strings.TrimSuffix(raw, "ms")
	case strings.HasSuffix(raw, "s"):
		inputUnit, numeric = "s", strings.TrimSuffix(raw, "s")
	}
	if n, err := strconv.ParseInt(numeric, 10, 64); err == nil {
		// Bound before unit conversion; never infer s/ms from digit count or overflow.
		minimum, maximum := int64(-62135596800), int64(253402300799)
		if inputUnit == "ms" {
			minimum *= 1000
			maximum = maximum*1000 + 999
		}
		if n < minimum || n > maximum {
			return time.Time{}, monitorapi.InputError(
				"invalid_time",
				"时间戳超出可表示日期范围；检查该工具的秒/毫秒单位，或显式使用 s/ms 后缀。",
			)
		}
		t := time.Unix(n, 0)
		if inputUnit == "ms" {
			t = time.UnixMilli(n)
		}
		return checkedTime(t, unit)
	}
	for _, layout := range wallTimeLayouts {
		wall, err := time.Parse(layout, raw)
		if err != nil {
			continue
		}
		t := time.Date(
			wall.Year(),
			wall.Month(),
			wall.Day(),
			wall.Hour(),
			wall.Minute(),
			wall.Second(),
			wall.Nanosecond(),
			location,
		)
		if !sameWallTime(t, wall) {
			return time.Time{}, monitorapi.InputError(
				"nonexistent_local_time",
				"该当地时刻因时区切换而不存在；请提供明确的 RFC3339 时间。",
			)
		}
		if ambiguousWallTime(t, wall, location) {
			return time.Time{}, monitorapi.InputError(
				"ambiguous_local_time",
				"该当地时刻对应两个时间点；请使用带 UTC 偏移的 RFC3339 时间。",
			)
		}
		return checkedTime(t, unit)
	}
	return time.Time{}, invalidTime()
}

func calendarTime(raw string, location *time.Location, reference time.Time) (string, bool) {
	keyword, clock, _ := strings.Cut(raw, " ")
	var days int
	switch keyword {
	case "today", "今天":
		days = 0
	case "yesterday", "昨天":
		days = -1
	default:
		return "", false
	}
	date := reference.In(location).AddDate(0, 0, days).Format("2006-01-02")
	if strings.TrimSpace(clock) != "" {
		date += " " + strings.TrimSpace(clock)
	}
	return date, true
}

func sameWallTime(a, b time.Time) bool {
	const layout = "2006-01-02T15:04:05.999999999"
	return a.Format(layout) == b.Format(layout)
}

func ambiguousWallTime(t, wall time.Time, location *time.Location) bool {
	_, offset := t.Zone()
	for _, shift := range []time.Duration{-24 * time.Hour, 24 * time.Hour} {
		_, other := t.Add(shift).In(location).Zone()
		if other != offset {
			candidate := t.Add(time.Duration(offset-other) * time.Second).In(location)
			if sameWallTime(candidate, wall) {
				return true
			}
		}
	}
	return false
}

func checkedTime(t time.Time, unit string) (time.Time, error) {
	if t.UTC().Year() < 1 || t.UTC().Year() > 9999 {
		return time.Time{}, invalidTime()
	}
	if t.Nanosecond()%int(timeResolution(unit)) != 0 {
		return time.Time{}, monitorapi.InputError("invalid_time_precision", "输入时间的精度超过该工具支持的整秒/整毫秒精度；不会静默舍入。")
	}
	return t, nil
}

func encodedTime(p param, t time.Time) (any, error) {
	n := t.Unix()
	if p.TimeUnit == "ms" {
		n = t.UnixMilli()
	}
	if p.Type == "integer" {
		return n, nil
	}
	if p.Type == "string" {
		return strconv.FormatInt(n, 10), nil
	}
	return nil, monitorapi.InputError("unsupported_time_contract", "工具的时间字段类型不受支持；需要复核工具目录。")
}

func invalidTime() error {
	return monitorapi.InputError(
		"invalid_time",
		"时间使用整型 Unix 时间戳（可带 s/ms 后缀）、YYYY-MM-DD HH:MM[:SS]、RFC3339、today/yesterday/now 或 今天/昨天/现在；不猜测模糊短语。",
	)
}
