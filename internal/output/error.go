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
	"fmt"
)

// ExitCodeUserError is the exit code for user/input errors.
const ExitCodeUserError = 1

// ExitCodeSystemError is the exit code for system/network errors.
const ExitCodeSystemError = 2

// CLIError is an error that carries an exit code for the process.
type CLIError struct {
	ExitCode int
	Code     string
	Message  string
}

func (e *CLIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// PrintError prints a structured error to stderr and returns a CLIError with the given exit code.
func PrintError(exitCode int, code, message, hint string) error {
	envelope := Err(code, message, hint)
	_ = envelope.PrintErr()
	return &CLIError{ExitCode: exitCode, Code: code, Message: message}
}

// UserError prints a user error (exit code 1) and returns a CLIError.
func UserError(code, message, hint string) error {
	return PrintError(ExitCodeUserError, code, message, hint)
}

// SystemError prints a system error (exit code 2) and returns a CLIError.
func SystemError(code, message, hint string) error {
	return PrintError(ExitCodeSystemError, code, message, hint)
}
