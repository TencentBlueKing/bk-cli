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
	"encoding/json"
	"fmt"
	"strings"
)

func expandPromQL(expression, namespace string) (string, error) {
	if strings.TrimSpace(expression) == "" {
		return "", fmt.Errorf("--promql is required")
	}
	if strings.Contains(expression, "{{namespace}}") {
		if namespace == "" {
			return "", fmt.Errorf("configure metrics_namespace or pass --namespace for {{namespace}}")
		}
		for _, quote := range []string{`"`, "'", "`"} {
			if strings.Contains(expression, quote+"{{namespace}}"+quote) {
				return "", fmt.Errorf(
					"use namespace={{namespace}} without quotes; the replacement is already quoted",
				)
			}
		}
		value, err := json.Marshal(namespace)
		if err != nil {
			return "", err
		}
		expression = strings.ReplaceAll(expression, "{{namespace}}", string(value))
	}
	return expression, nil
}
