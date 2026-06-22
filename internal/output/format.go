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

package output

import (
	"os"

	"golang.org/x/term"
)

// Format represents output format options.
type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
	FormatAuto Format = ""
)

// ResolveFormat determines the output format based on user preference.
func ResolveFormat(override string) Format {
	switch Format(override) {
	case FormatJSON:
		return FormatJSON
	case FormatText:
		return FormatText
	default:
		return FormatJSON
	}
}

// IsInteractive returns true if stdout is an interactive terminal.
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
