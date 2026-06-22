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

// Package main is the bk-cli entrypoint.
package main

import (
	"errors"
	"os"

	"github.com/TencentBlueKing/bk-cli/cmd"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

// Build metadata is set at build time via -ldflags.
var (
	version   = "dev"
	commitID  = "unknown"
	buildTime = "unknown"
)

func main() {
	cmd.SetBuildInfo(cmd.BuildInfo{
		Version:   version,
		CommitID:  commitID,
		BuildTime: buildTime,
	})
	if err := cmd.Execute(); err != nil {
		var cliErr *output.CLIError
		if errors.As(err, &cliErr) {
			os.Exit(cliErr.ExitCode)
		}
		// Non-CLIError (e.g. cobra flag parsing errors) — emit JSON envelope
		// so the agent-first contract is never violated (silent exit = broken).
		reportedErr := output.UserError("command_error", err.Error(), "Run with --help for usage")
		if errors.As(reportedErr, &cliErr) {
			os.Exit(cliErr.ExitCode)
		}
		os.Exit(1)
	}
}
