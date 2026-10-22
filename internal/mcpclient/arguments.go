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
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// NamedArgument preserves occurrence order so repeated flags can be rejected.
// The command layer must validate CLI token structure before passing values.
type NamedArgument struct{ Name, Value string }

// Parameter describes a schema property that can safely become a CLI flag.
type Parameter struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

// ReadArguments reads one complete arguments object from inline JSON, @file, or
// stdin (-). The caller must skip this function for help. Paths are cwd-relative.
func ReadArguments(source string, stdin io.Reader) (map[string]any, error) {
	var reader io.Reader = strings.NewReader(source)
	if source == "-" {
		if stdin == nil {
			return nil, argumentError()
		}
		reader = stdin
	} else if path, ok := strings.CutPrefix(source, "@"); ok {
		file, err := os.Open(path)
		if err != nil {
			return nil, argumentError()
		}
		defer func() { _ = file.Close() }()
		reader = file
	}
	raw, err := decodeJSON(reader)
	if err != nil {
		return nil, argumentError()
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, argumentError()
	}
	return object, nil
}

var parameterName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// ReservedParameter reports names owned or reserved by the MCP CLI contract.
func ReservedParameter(name string) bool {
	switch name {
	case "help", "h", "check", "tools", "config", "body", "body-schema", "timeout", "jq", "context",
		"dry-run", "verbose", "v", "insecure", "format", "header", "stage", "tenant", "quiet", "q":
		return true
	}
	return false
}

// Parameters returns stable, unambiguous flag definitions. All other properties
// remain available in complete JSON input; no schema defaults are injected.
func Parameters(schema map[string]any) []Parameter {
	if complexSchema(schema) {
		return nil
	}
	properties, _ := schema["properties"].(map[string]any)
	result := make([]Parameter, 0, len(properties))
	for name, raw := range properties {
		property, ok := raw.(map[string]any)
		if !ok || !parameterName.MatchString(name) || ReservedParameter(name) || complexSchema(property) {
			continue
		}
		kind, _ := property["type"].(string)
		switch kind {
		case "string", "integer", "number", "boolean", "array", "object":
		default:
			continue
		}
		description, _ := property["description"].(string)
		result = append(
			result,
			Parameter{Name: name, Type: kind, Description: description, Required: required(schema, name)},
		)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func complexSchema(schema map[string]any) bool {
	for _, key := range []string{"$ref", "$dynamicRef", "oneOf", "anyOf", "allOf", "not", "if", "then", "else"} {
		if _, ok := schema[key]; ok {
			return true
		}
	}
	return false
}

func required(schema map[string]any, name string) bool {
	names, _ := schema["required"].([]any)
	for _, value := range names {
		if value == name {
			return true
		}
	}
	return false
}

// BuildArguments maps explicit values and validates the final object. A non-nil
// body completely overrides recognized named values; names must still be valid
// and unique. Schemas and bodies must come from lossless JSON decoding.
func BuildArguments(schema map[string]any, named []NamedArgument, body map[string]any) (map[string]any, error) {
	parameters := make(map[string]Parameter)
	for _, parameter := range Parameters(schema) {
		parameters[parameter.Name] = parameter
	}
	seen := make(map[string]bool)
	arguments := make(map[string]any)
	for _, argument := range named {
		parameter, ok := parameters[argument.Name]
		if !ok || seen[argument.Name] {
			return nil, argumentError()
		}
		seen[argument.Name] = true
		if body != nil {
			continue
		}
		if parameter.Type == "string" && !utf8.ValidString(argument.Value) {
			return nil, argumentError()
		}
		var value any = argument.Value
		if parameter.Type != "string" {
			var err error
			value, err = decodeJSON(strings.NewReader(argument.Value))
			if err != nil {
				return nil, argumentError()
			}
		}
		arguments[argument.Name] = value
	}
	if body != nil {
		arguments = body
	}
	if !validateValue(schema, arguments, 0) {
		return nil, argumentError()
	}
	return arguments, nil
}
