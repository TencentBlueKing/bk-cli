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

package systemcmd

import (
	"fmt"
	"strings"

	"github.com/TencentBlueKing/bk-cli/internal/output"
)

// ValidatePositiveIntFlag validates that a required integer flag is greater than zero.
func ValidatePositiveIntFlag(flagName string, value int) error {
	if value <= 0 {
		return output.UserError(
			"invalid_argument",
			flagName+" must be greater than 0",
			fmt.Sprintf("Use --%s with a positive integer", flagName),
		)
	}
	return nil
}

// ValidatePositiveIntFlagIfChanged validates an optional positive integer flag only when provided.
func ValidatePositiveIntFlagIfChanged(flagName string, value int, changed bool) error {
	if !changed {
		return nil
	}
	return ValidatePositiveIntFlag(flagName, value)
}

// ValidateNonNegativeIntFlag validates that an integer flag is zero or greater.
func ValidateNonNegativeIntFlag(flagName string, value int) error {
	if value < 0 {
		return output.UserError(
			"invalid_argument",
			flagName+" must be greater than or equal to 0",
			fmt.Sprintf("Use --%s with a non-negative integer", flagName),
		)
	}
	return nil
}

// ValidateNonEmptyStringFlag validates that a required string flag is not empty after trimming.
func ValidateNonEmptyStringFlag(flagName, value string) error {
	if strings.TrimSpace(value) == "" {
		return output.UserError(
			"invalid_argument",
			flagName+" cannot be empty",
			fmt.Sprintf("Use --%s with a non-empty value", flagName),
		)
	}
	return nil
}
