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
	"embed"
	"errors"
	"os"

	"github.com/TencentBlueKing/bk-cli/cmd"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	"github.com/TencentBlueKing/bk-cli/internal/plugin"
)

// Build metadata is set at build time via -ldflags.
var (
	version   = "dev"
	commitID  = "unknown"
	buildTime = "unknown"
)

//go:embed skills/*/SKILL.md skills/*/references/*
var skillsFS embed.FS

func main() {
	cmd.SetBuildInfo(cmd.BuildInfo{
		Version:   version,
		CommitID:  commitID,
		BuildTime: buildTime,
	})
	cmd.SetSkillsFS(skillsFS)
	if code := exitCode(cmd.Execute()); code != 0 {
		os.Exit(code)
	}
}

// exitCode maps an execution result to the process exit code, printing an envelope only for
// errors that have not been reported yet. Plugin exit statuses were already reported by the child.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var child *plugin.ExitStatus
	if errors.As(err, &child) {
		return child.Code
	}
	var cliErr *output.CLIError
	if errors.As(err, &cliErr) {
		return cliErr.ExitCode
	}
	reported := output.UserError("command_error", err.Error(), "Run with --help for usage")
	if errors.As(reported, &cliErr) {
		return cliErr.ExitCode
	}
	return 1
}
