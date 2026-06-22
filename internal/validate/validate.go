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

// Package validate contains shared input validators for CLI-facing fields.
package validate

import (
	"fmt"
	"regexp"
)

var (
	gatewayNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{2,29}$`)
	contextNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// FieldError identifies a validation failure for a specific input field.
type FieldError struct {
	Field   string
	Value   string
	Pattern string
}

func (e *FieldError) Error() string {
	if e.Pattern == "" {
		return fmt.Sprintf("%s %q is invalid", e.Field, e.Value)
	}
	return fmt.Sprintf("%s %q must match %s", e.Field, e.Value, e.Pattern)
}

// ValidateGatewayName ensures a gateway name matches the CLI contract.
func ValidateGatewayName(name string) error {
	if !gatewayNameRe.MatchString(name) {
		return &FieldError{Field: "gateway_name", Value: name, Pattern: gatewayNameRe.String()}
	}
	return nil
}

// ValidateContextName ensures a context name is a safe single-segment slug.
func ValidateContextName(name string) error {
	if name == "." || name == ".." || !contextNameRe.MatchString(name) {
		return &FieldError{Field: "context name", Value: name, Pattern: contextNameRe.String()}
	}
	return nil
}

// ValidateHeaderName ensures a header name uses RFC token characters only.
func ValidateHeaderName(name string) error {
	if name == "" {
		return fmt.Errorf("header name cannot be empty")
	}

	for i := range len(name) {
		if !isHeaderTokenChar(name[i]) {
			return fmt.Errorf("invalid header name %q", name)
		}
	}
	return nil
}

// ValidateHeaderValue rejects line breaks and illegal control characters.
func ValidateHeaderValue(value string) error {
	for i := range len(value) {
		b := value[i]
		if b == '\r' || b == '\n' || ((b < 0x20 && b != '\t') || b == 0x7f) {
			return fmt.Errorf("invalid header value %q", value)
		}
	}
	return nil
}

func isHeaderTokenChar(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z':
		return true
	case b >= 'A' && b <= 'Z':
		return true
	case b >= '0' && b <= '9':
		return true
	}

	switch b {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	default:
		return false
	}
}
