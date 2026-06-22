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

// Package utils provides small shared parsing helpers for internal packages.
package utils

import (
	"net"
	"strconv"
	"strings"

	"github.com/TencentBlueKing/bk-cli/internal/output"
)

// ParseCSVFields returns the non-empty trimmed fields from a comma-separated string.
func ParseCSVFields(raw string) []string {
	parts := strings.Split(raw, ",")
	fields := make([]string, 0, len(parts))
	for _, part := range parts {
		field := strings.TrimSpace(part)
		if field == "" {
			continue
		}
		fields = append(fields, field)
	}
	return fields
}

// ParseCSVInts parses a comma-separated list of positive integers.
func ParseCSVInts(raw, flagName, hint string) ([]int, error) {
	parts := strings.Split(raw, ",")
	values := make([]int, 0, len(parts))

	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			return nil, output.UserError(
				"invalid_argument",
				flagName+" must be a comma-separated list of integers",
				hint,
			)
		}

		value, err := strconv.Atoi(token)
		if err != nil || value <= 0 {
			return nil, output.UserError(
				"invalid_argument",
				flagName+" must be a comma-separated list of integers",
				hint,
			)
		}
		values = append(values, value)
	}

	if len(values) == 0 {
		return nil, output.UserError(
			"invalid_argument",
			flagName+" must be a comma-separated list of integers",
			hint,
		)
	}

	return values, nil
}

// IsIPv4 reports whether the value is a valid IPv4 address.
func IsIPv4(value string) bool {
	ip := net.ParseIP(value)
	return ip != nil && ip.To4() != nil
}

// FormatCommandExamples builds Cobra Example text with consistent indentation.
func FormatCommandExamples(lines ...string) string {
	return "  " + strings.Join(lines, "\n  ")
}
