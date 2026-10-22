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

package mcpclient

import (
	"encoding/json"
	"math/big"
	"reflect"
	"strconv"
	"strings"
)

// Numeric bounds constrain validation cost; accepted numbers retain their wire
// spelling. This is not a floating-point range limit.
const (
	maxNumberChars    = 4096
	maxNumberExponent = 10000
)

func numberValue(number json.Number) (*big.Rat, bool) {
	text := number.String()
	if len(text) > maxNumberChars {
		return nil, false
	}
	if index := strings.IndexAny(text, "eE"); index >= 0 {
		exponent, err := strconv.Atoi(text[index+1:])
		if err != nil || exponent > maxNumberExponent || exponent < -maxNumberExponent {
			return nil, false
		}
	}
	value, ok := new(big.Rat).SetString(text)
	return value, ok
}

// validateValue deliberately checks only explicit types, required properties,
// enums, object properties and array items. Remote references are never fetched.
func validateValue(schema map[string]any, value any, depth int) bool {
	if depth > maxJSONDepth {
		return false
	}
	kind, _ := schema["type"].(string)
	if !matchesType(kind, value) {
		return false
	}
	if enum, ok := schema["enum"].([]any); ok {
		found := false
		for _, candidate := range enum {
			if equalJSON(candidate, value) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	switch actual := value.(type) {
	case map[string]any:
		names, _ := schema["required"].([]any)
		for _, name := range names {
			text, ok := name.(string)
			if !ok {
				continue
			}
			if _, exists := actual[text]; !exists {
				return false
			}
		}
		properties, _ := schema["properties"].(map[string]any)
		for name, element := range actual {
			property, _ := properties[name].(map[string]any)
			if !validateValue(property, element, depth+1) {
				return false
			}
		}
	case []any:
		items, _ := schema["items"].(map[string]any)
		for _, element := range actual {
			if !validateValue(items, element, depth+1) {
				return false
			}
		}
	}
	return true
}

func matchesType(kind string, value any) bool {
	switch kind {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "null":
		return value == nil
	case "number", "integer":
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		rational, ok := numberValue(number)
		return ok && (kind != "integer" || rational.IsInt())
	default:
		return true
	}
}

func equalJSON(left, right any) bool {
	switch value := left.(type) {
	case json.Number:
		other, ok := right.(json.Number)
		if !ok {
			return false
		}
		l, lok := numberValue(value)
		r, rok := numberValue(other)
		return lok && rok && l.Cmp(r) == 0
	case map[string]any:
		other, ok := right.(map[string]any)
		if !ok || len(value) != len(other) {
			return false
		}
		for name, element := range value {
			candidate, exists := other[name]
			if !exists || !equalJSON(element, candidate) {
				return false
			}
		}
		return true
	case []any:
		other, ok := right.([]any)
		if !ok || len(value) != len(other) {
			return false
		}
		for index, element := range value {
			if !equalJSON(element, other[index]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(left, right)
	}
}
