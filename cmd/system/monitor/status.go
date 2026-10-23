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
	"os"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
	"github.com/TencentBlueKing/bk-cli/internal/config"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
	"github.com/TencentBlueKing/bk-cli/internal/update"
)

func newStatusCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var offline, checkUpdate bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check local setup, authenticated Monitor API, catalog version and optional profile",
		Long: `Check local context and credential-file presence, then make a small authenticated
Monitor schema request. With --profile, also preflight its configured resources.
No log, alert or metric payload is read. --offline and --dry-run never use the network.
--check_update optionally queries the official GitHub release source; it never installs anything.`,
		Example: "  bk-cli monitor status --profile game-prod\n" +
			"  bk-cli monitor status --offline\n  bk-cli monitor status --check_update",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bound, err := selectProfile(cmd, deps)
			if err != nil {
				return printMonitorFailure(cmd, err)
			}
			if p := currentProfile(cmd); p != nil && p.Version == 2 {
				return runV2Report(cmd, bound, p, true, offline, checkUpdate)
			}
			dry := bound.IsDryRun != nil && bound.IsDryRun()
			version := cmd.Root().Annotations["build_version"]
			if version == "" {
				version = "dev"
			}
			checks := []map[string]any{{"name": "cli_version", "status": "info", "version": version}}
			exit := 0
			fail := func(name string, err error) {
				row, code := diagnosticFailure(name, err)
				checks = append(checks, row)
				if code > exit {
					exit = code
				}
			}
			contextName := ""
			if bound.GetContext != nil {
				contextName = bound.GetContext()
			}
			selected, cfg, err := config.ResolveContextReadOnly(contextName)
			if err != nil || cfg == nil {
				fail(
					"context",
					monitorapi.InputError(
						"config_error",
						"context 不存在或不可读；先运行 bk-cli context list。",
					),
				)
			} else {
				checks = append(
					checks,
					map[string]any{"name": "context", "status": "pass", "context": selected},
				)
				if _, err := os.Stat(config.CredentialsPath(selected)); err != nil {
					fail(
						"credential_file",
						monitorapi.InputError(
							"auth_required",
							"没有可读凭据文件；请在该 context 下运行 bk-cli auth login。",
						),
					)
				} else {
					checks = append(
						checks,
						map[string]any{
							"name":    "credential_file",
							"status":  "pass",
							"meaning": "仅证明文件存在，不证明 Token 有效。",
						},
					)
				}
			}
			if !offline && !dry && exit == 0 {
				// Explicit read-only resolution above avoids changing the global active context.
				bound.GetContext = func() string { return selected }
				rt, err := systemcmd.ResolveRuntime(bound)
				if err != nil {
					fail("runtime", err)
				} else {
					rt := monitorapi.Bind(cmd, rt)
					reply, err := monitorapi.Call(
						rt,
						"lookup_tool_schema",
						map[string]any{"tool_name": "search_spaces"},
					)
					if err != nil {
						fail("authenticated_api", err)
					} else {
						remote, _ := monitorapi.Map(reply.Data)["catalog_version"].(string)
						checks = append(
							checks,
							map[string]any{
								"name":       "authenticated_api",
								"status":     "pass",
								"request_id": reply.RequestID,
							},
						)
						state := "pass"
						if remote == "" || remote != catalogData.Version {
							state = "warn"
						}
						checks = append(checks, map[string]any{
							"name":    "catalog",
							"status":  state,
							"local":   catalogData.Version,
							"remote":  remote,
							"meaning": "目录版本不一致仅提示需复核快照，不证明工具不兼容。",
						})
						if p := currentProfile(cmd); p != nil {
							rows, code := checkProfile(rt, p)
							checks = append(checks, rows...)
							if code > exit {
								exit = code
							}
						}
					}
				}
			} else {
				checks = append(
					checks,
					map[string]any{
						"name":    "authenticated_api",
						"status":  "skipped",
						"meaning": "离线、dry-run 或本地检查失败；未验证鉴权和接口。",
					},
				)
			}
			updateCheck := map[string]any{
				"name":   "update",
				"status": "skipped",
				"next":   "bk-cli monitor status --check_update",
			}
			if checkUpdate && !offline && !dry {
				source := &update.GitHubSource{Owner: "TencentBlueKing", Repo: "bk-cli"}
				release, err := source.CheckLatest(cmd.Context())
				updateCheck = releaseCheck(version, release, err)
			}
			checks = append(checks, updateCheck)
			return writeDiagnostic(cmd, map[string]any{
				"ok": exit == 0, "offline": offline, "dry_run": dry, "checks": checks,
				"meaning": "本地检查通过不等于有业务权限；离线或 skipped 项尚未验证。",
			}, exit)
		},
	}
	addProfileFlags(cmd)
	cmd.Flags().BoolVar(&offline, "offline", false, "Only inspect local state; no network or credential decryption")
	cmd.Flags().BoolVar(
		&checkUpdate,
		"check_update",
		false,
		"Check official GitHub release version without downloading or installing",
	)
	return cmd
}

func releaseCheck(version string, release *update.ReleaseInfo, err error) map[string]any {
	if err != nil || release == nil {
		return map[string]any{"name": "update", "status": "warn", "decision": "update_check_unavailable"}
	}
	row := map[string]any{
		"name": "update", "status": "pass",
		"current_version": version, "latest_version": release.Version,
	}
	if !semver.IsValid("v"+strings.TrimPrefix(version, "v")) ||
		!semver.IsValid("v"+strings.TrimPrefix(release.Version, "v")) {
		row["status"], row["decision"] = "warn", "version_not_comparable"
		return row
	}
	newer := update.IsNewer(version, release.Version)
	row["newer_release"] = newer
	if newer {
		row["status"], row["decision"] = "warn", "newer_release_available"
	}
	return row
}
