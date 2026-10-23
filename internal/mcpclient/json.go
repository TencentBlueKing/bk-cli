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

// Package mcpclient provides context-independent configuration and input handling
// for remote MCP servers. Transport integration is intentionally separate.
package mcpclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

// MaxInputBytes bounds local configuration and arguments before decoding.
const MaxInputBytes = 16 << 20

const maxJSONDepth = 128

// Error is a safe, machine-readable local MCP error. Messages never contain
// configuration values or JSON parser errors that could disclose credentials.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func configError() error { return &Error{"invalid_mcp_config", "Invalid MCP configuration"} }

func argumentError() error { return &Error{"invalid_mcp_arguments", "Invalid MCP arguments"} }

// decodeJSON keeps number literals and rejects duplicate keys at every depth.
func decodeJSON(reader io.Reader) (any, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxInputBytes+1))
	if err != nil || len(data) > MaxInputBytes || !utf8.Valid(data) {
		return nil, fmt.Errorf("cannot read bounded JSON input")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := decodeValue(decoder, 0)
	if err != nil {
		return nil, err
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("unexpected trailing JSON")
	}
	return value, nil
}

func decodeValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > maxJSONDepth {
		return nil, fmt.Errorf("JSON nesting limit exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON")
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delimiter {
	case '{':
		result := make(map[string]any)
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, fmt.Errorf("invalid JSON key")
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("invalid JSON key")
			}
			if _, exists := result[name]; exists {
				return nil, fmt.Errorf("duplicate JSON key")
			}
			value, err := decodeValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			result[name] = value
		}
		if _, err := decoder.Token(); err != nil {
			return nil, fmt.Errorf("invalid JSON object")
		}
		return result, nil
	case '[':
		result := make([]any, 0)
		for decoder.More() {
			value, err := decodeValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, fmt.Errorf("invalid JSON array")
		}
		return result, nil
	default:
		return nil, fmt.Errorf("invalid JSON delimiter")
	}
}
