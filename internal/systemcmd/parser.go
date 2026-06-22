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

	json "github.com/goccy/go-json"

	"github.com/TencentBlueKing/bk-cli/internal/output"
)

// ParseJSONObjectFlag validates that a required string flag contains a JSON object and decodes it.
func ParseJSONObjectFlag(flagName, raw string) (map[string]any, error) {
	if err := ValidateNonEmptyStringFlag(flagName, raw); err != nil {
		return nil, err
	}

	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, output.UserError(
			"invalid_argument",
			flagName+" must be a valid JSON object",
			fmt.Sprintf(`Use --%s '{"key":"value"}'`, flagName),
		)
	}

	object, ok := decoded.(map[string]any)
	if !ok {
		return nil, output.UserError(
			"invalid_argument",
			flagName+" must be a JSON object",
			fmt.Sprintf(`Use --%s '{"key":"value"}'`, flagName),
		)
	}

	return object, nil
}
