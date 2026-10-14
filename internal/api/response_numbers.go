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

package api

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

func isNumberByte(b byte) bool {
	return b == '-' || b == '+' || b == '.' || b == 'e' || b == 'E' || (b >= '0' && b <= '9')
}

// normalizeResponseNumbers retains float64 for ordinary response numbers while
// keeping json.Number wherever float conversion could change the emitted value.
func normalizeResponseNumbers(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			typed[key] = normalizeResponseNumbers(item)
		}
	case []any:
		for index, item := range typed {
			typed[index] = normalizeResponseNumbers(item)
		}
	case json.Number:
		return normalizeResponseNumber(typed)
	}
	return value
}

// responseNeedsNumber scans validated JSON without allocating a second object
// tree. Strings (including escaped quotes and numeric-looking keys) are skipped.
func responseNeedsNumber(body []byte) bool {
	for i := 0; i < len(body); i++ {
		if body[i] == '"' {
			for {
				end := bytes.IndexByte(body[i+1:], '"')
				if end < 0 {
					return true
				} // Let the standard decoder reject bad strings.
				i += end + 1
				escaped := false
				for j := i - 1; j >= 0 && body[j] == '\\'; j-- {
					escaped = !escaped
				}
				if !escaped {
					break
				}
			}
			continue
		}
		if body[i] != '-' && (body[i] < '0' || body[i] > '9') {
			continue
		}
		start := i
		for i < len(body) && isNumberByte(body[i]) {
			i++
		}
		raw := string(body[start:i])
		// A decimal with at most 15 significant digits in the normal float
		// range uniquely round-trips. Short, non-exponent literals meet that
		// condition without parsing and formatting the same number twice.
		if len(strings.TrimPrefix(raw, "-")) <= 15 && !strings.ContainsAny(raw, "eE") {
			i--
			continue
		}
		if _, ok := responseNumberFloat(raw); !ok {
			return true
		}
		i--
	}
	return false
}

func normalizeResponseNumber(number json.Number) any {
	if f, ok := responseNumberFloat(number.String()); ok {
		return f
	}
	return number
}

func responseNumberFloat(raw string) (float64, bool) {
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	if !strings.ContainsAny(raw, ".eE") && math.Abs(f) <= 1<<53-1 {
		return f, true
	}
	// Compare decimal values, not their spelling (2.0 and 2e0 both mean 2).
	// A fixed buffer and digit spans avoid arbitrary-precision allocations for
	// every ordinary metric sample. Only values that round-trip stay float64.
	var buffer [32]byte
	encoded := strconv.AppendFloat(buffer[:0], f, 'g', -1, 64)
	return f, sameDecimal(raw, string(encoded))
}

type decimalValue struct {
	digits   string
	scale    int64
	negative bool
}

func decimalParts(raw string) (decimalValue, bool) {
	value := decimalValue{negative: strings.HasPrefix(raw, "-")}
	if value.negative {
		raw = raw[1:]
	}
	mantissa, exponent, hasExponent := strings.Cut(raw, "e")
	if !hasExponent {
		mantissa, exponent, hasExponent = strings.Cut(raw, "E")
	}
	dot := strings.IndexByte(mantissa, '.')
	start, end := 0, len(mantissa)
	for start < end && (mantissa[start] == '0' || mantissa[start] == '.') {
		start++
	}
	if start == end {
		return decimalValue{}, true
	}
	for end > start && (mantissa[end-1] == '0' || mantissa[end-1] == '.') {
		end--
	}
	if hasExponent {
		var err error
		value.scale, err = strconv.ParseInt(exponent, 10, 64)
		if err != nil {
			return decimalValue{}, false
		}
	}
	// For a nonzero finite float the exponent is bounded by the literal length;
	// a huge exponent cannot round-trip. Bound it before adding length offsets.
	if value.scale > int64(len(raw))+400 || value.scale < -int64(len(raw))-400 {
		return decimalValue{}, false
	}
	if dot >= 0 {
		value.scale -= int64(len(mantissa) - dot - 1)
	}
	trailing := len(mantissa) - end
	if dot >= end {
		trailing--
	}
	value.scale += int64(trailing)
	value.digits = mantissa[start:end]
	return value, true
}

func sameDecimal(left, right string) bool {
	a, ok := decimalParts(left)
	if !ok {
		return false
	}
	b, ok := decimalParts(right)
	if !ok || a.scale != b.scale || a.negative != b.negative {
		return false
	}
	// Significant digits may contain a decimal point; compare without copying.
	i, j := 0, 0
	for i < len(a.digits) && j < len(b.digits) {
		if a.digits[i] == '.' {
			i++
			continue
		}
		if b.digits[j] == '.' {
			j++
			continue
		}
		if a.digits[i] != b.digits[j] {
			return false
		}
		i++
		j++
	}
	return i == len(a.digits) && j == len(b.digits)
}
