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
	"errors"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newProfileSpec() systemcmd.SystemSpec {
	return systemcmd.SystemSpec{
		Name: "profile", Description: "Inspect and check SRE-maintained observability profiles",
		RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
			parent.AddCommand(newProfileListCmd(), newProfileShowCmd(deps), newProfileCheckCmd(deps))
			return nil
		},
	}
}

func newProfileListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "list", Short: "List profile names and scopes locally, without credentials or network",
		Example: "  bk-cli monitor profile list --profile_file ./profiles.yaml", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pathFlag, _ := cmd.Flags().GetString("profile_file")
			path, err := profileFilePath(pathFlag)
			if err != nil {
				return printMonitorFailure(cmd, err)
			}
			file, hash, err := loadProfiles(path)
			if err != nil {
				return printMonitorFailure(cmd, err)
			}
			rows := []map[string]any{}
			for _, name := range sortedProfileNames(file) {
				p := file.Profiles[name]
				if file.Version == 2 {
					rows = append(
						rows,
						map[string]any{
							"name":        name,
							"description": p.Description,
							"environment": p.Environment,
							"components":  sortedKeys(p.Components),
						},
					)
					continue
				}
				rows = append(
					rows,
					map[string]any{
						"name":        name,
						"description": p.Description,
						"context":     p.Context,
						"bk_biz_id":   p.BizID,
					},
				)
			}
			return output.WriteCommandEnvelope(
				cmd,
				output.SuccessData(
					map[string]any{
						"version":  file.Version,
						"file":     path,
						"sha256":   hash,
						"profiles": rows,
					},
				),
			)
		},
	}
	output.AddJQFlag(cmd)
	cmd.Flags().String("profile_file", "", "Profiles YAML file (or BK_CLI_MONITOR_PROFILE_FILE)")
	return cmd
}

func newProfileShowCmd(deps systemcmd.BuildDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "show",
		Short:   "Show one resolved profile locally; no runtime, credentials or network needed",
		Example: "  bk-cli monitor profile show --profile game-prod --profile_file ./profiles.yaml",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := selectProfile(cmd, deps)
			if err != nil {
				return printMonitorFailure(cmd, err)
			}
			p := currentProfile(cmd)
			if p == nil {
				return printMonitorFailure(
					cmd,
					monitorapi.InputError("needs_profile", "请用 --profile 指定名称。"),
				)
			}
			if p.Version == 2 {
				return writeMonitorEnvelope(cmd, output.SuccessData(v2ProfileView(p)))
			}
			return writeMonitorEnvelope(cmd, output.SuccessData(p.Profile))
		},
	}
	addProfileFlags(cmd)
	return cmd
}

func newProfileCheckCmd(deps systemcmd.BuildDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Preflight configured resources and permissions without reading logs or metrics",
		Long: `Check a representative read-only tool and metadata for each configured resource.
This does not read log bodies or metric series. It is not a cached grant or validation
of every query/filter; the server authorizes each real call again.`,
		Example: "  bk-cli monitor profile check --profile game-prod",
		Args:    cobra.NoArgs,
		RunE: runMonitor(deps, func(cmd *cobra.Command, rt *monitorapi.Runtime) error {
			p := currentProfile(cmd)
			if p == nil {
				return monitorapi.InputError("needs_profile", "请用 --profile 指定名称。")
			}
			if rt.DryRun {
				reply, err := monitorapi.Call(
					rt,
					"lookup_tool",
					map[string]any{
						"tool_name":      profileProbeTools(p.Profile)[0],
						"bk_biz_id":      p.Profile.BizID,
						"available_only": false,
					},
				)
				return printReply(cmd, reply, err)
			}
			rows, exit := checkProfile(rt, p)
			return writeDiagnostic(
				cmd,
				map[string]any{
					"ok":      exit == 0,
					"checks":  rows,
					"meaning": "预检不读取日志或指标，不缓存授权，也不证明所有查询和过滤条件有效。",
				},
				exit,
			)
		}),
	}
	addProfileFlags(cmd)
	return cmd
}

func profileProbeTools(p queryProfile) []string {
	names := []string{}
	if p.Log != nil {
		if p.Log.Scene != "" {
			names = append(names, "search_logs")
		} else {
			names = append(names, "get_index_set_fields")
		}
	}
	if p.Metric != nil {
		names = append(names, "list_time_series_metrics")
	}
	if p.APM != nil {
		names = append(names, "list_apm_services")
	}
	if len(names) == 0 {
		names = append(names, "list_index_sets")
	}
	return names
}

func checkProfile(rt *monitorapi.Runtime, p *selectedProfile) ([]map[string]any, int) {
	rows := []map[string]any{}
	exit := 0
	for _, name := range profileProbeTools(p.Profile) {
		values := map[string]any{}
		// Work on a copy; check must not claim these were applied to a business query.
		copy := *p
		copy.Applied = nil
		err := profileArgs(&copy, toolsByName[name], values, "", nil)
		var result map[string]any
		if err == nil {
			result, err = check(rt, p.Profile.BizID, name, values)
		}
		if err == nil {
			// Permission for metric/scene tools can be business-scoped. Read only
			// resource metadata as well; a granted business is not proof a table exists.
			metadataTool := name
			if name == "search_logs" {
				metadataTool = "get_scene_log_fields"
			}
			if p.Profile.Log != nil || p.Profile.Metric != nil || p.Profile.APM != nil {
				metadataArgs := map[string]any{}
				err = profileArgs(&copy, toolsByName[metadataTool], metadataArgs, "", nil)
				if err == nil {
					var reply *monitorapi.Reply
					reply, err = monitorapi.Execute(rt, metadataTool, metadataArgs)
					if err == nil {
						rows = append(rows, map[string]any{
							"tool":          name,
							"status":        "pass",
							"decision":      "ready",
							"steps":         result["steps"],
							"metadata_tool": metadataTool,
							"request_id":    reply.RequestID,
						})
					}
				}
			} else {
				rows = append(
					rows,
					map[string]any{
						"tool":     name,
						"status":   "pass",
						"decision": "ready",
						"steps":    result["steps"],
					},
				)
			}
			if err == nil {
				continue
			}
		}
		row, code := diagnosticFailure(name, err)
		row["tool"] = name
		row["next"] = profileDiagnosticNext(row, name, p)
		rows = append(rows, row)
		if code > exit {
			exit = code
		}
	}
	return rows, exit
}

// Diagnostics always print their report, and return a nonzero exit if any check failed.
func writeDiagnostic(cmd *cobra.Command, data map[string]any, exit int) error {
	if err := writeMonitorEnvelope(cmd, &output.Envelope{OK: exit == 0, Data: data}); err != nil {
		return err
	}
	if exit != 0 {
		return &output.CLIError{Code: "monitor_check_failed", Message: "一个或多个检查失败；查看 checks。", ExitCode: exit}
	}
	return nil
}

func diagnosticFailure(name string, err error) (map[string]any, int) {
	row := map[string]any{"name": name, "status": "fail", "decision": "request_failed"}
	var decision *monitorapi.Decision
	if errors.As(err, &decision) {
		row["decision"], row["message"] = decision.Code, decision.Message
		row["request_id"], row["next"] = decision.RequestID, decision.Next
		if len(decision.AnyOf) > 0 {
			row["any_of_permissions"] = decision.AnyOf
		}
		return row, decision.ExitCode
	}
	var cli *output.CLIError
	if errors.As(err, &cli) {
		row["decision"] = cli.Code
		return row, cli.ExitCode
	}
	// Never include upstream error text or credentials in diagnostics.
	return row, output.ExitCodeSystemError
}
