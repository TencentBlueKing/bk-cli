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

package requestexec

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	bodyFilePrefix = "@"
	bodyStdinToken = "-"
)

func resolveBodyJSON(body string) (string, error) {
	switch {
	case body == bodyStdinToken:
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read --body stdin: %w", err)
		}
		return bodyDataString("stdin", data)
	case strings.HasPrefix(body, bodyFilePrefix):
		path := strings.TrimPrefix(body, bodyFilePrefix)
		if path == "" {
			return "", fmt.Errorf("--body @file path is required")
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("failed to read --body file %q: %w", path, err)
		}
		return bodyDataString(fmt.Sprintf("file %q", path), data)
	default:
		return body, nil
	}
}

func bodyDataString(source string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("--body %s is empty", source)
	}

	return string(data), nil
}
