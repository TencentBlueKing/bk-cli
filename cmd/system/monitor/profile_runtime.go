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
	"context"
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

type (
	profileContextKey struct{}
	runtimeContextKey struct{}
)

func addProfileFlags(cmd *cobra.Command) {
	if cmd.Name() == "show" {
		output.AddJQFlag(cmd)
	} else {
		var stage string
		var headers []string
		systemcmd.AddCommonRequestFlagsWithoutBody(cmd, &stage, &headers)
	}
	cmd.Flags().String("profile", "", "Named SRE-maintained observability profile (no credentials)")
	cmd.Flags().String("component", "", "V2 business component; required when the profile has multiple components")
	cmd.Flags().String(
		"binding",
		"",
		"V2 named observability binding; required when this module has multiple candidates",
	)
	cmd.Flags().String(
		"profile_file",
		"",
		"Profiles YAML file; defaults to BK_CLI_MONITOR_PROFILE_FILE or config-dir/monitor/profiles.yaml",
	)
}

func currentProfile(cmd *cobra.Command) *selectedProfile {
	if cmd.Context() == nil {
		return nil
	}
	selection, _ := cmd.Context().Value(profileContextKey{}).(*selectedProfile)
	return selection
}

func selectProfile(cmd *cobra.Command, deps systemcmd.BuildDeps) (systemcmd.BuildDeps, error) {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cmd.SetContext(context.WithValue(ctx, profileContextKey{}, (*selectedProfile)(nil)))
	name, _ := cmd.Flags().GetString("profile")
	fileFlag, _ := cmd.Flags().GetString("profile_file")
	if name == "" {
		if cmd.Flags().Changed("profile") || fileFlag != "" || cmd.Flags().Changed("component") ||
			cmd.Flags().Changed("binding") {
			return deps, monitorapi.InputError(
				"needs_profile",
				"请用 --profile 选择名字；查看可用配置：bk-cli monitor profile list --profile_file FILE。",
			)
		}
		return deps, nil
	}
	path, err := profileFilePath(fileFlag)
	if err != nil {
		return deps, err
	}
	file, hash, err := loadProfiles(path)
	if err != nil {
		return deps, err
	}
	p, ok := file.Profiles[name]
	if !ok {
		return deps, monitorapi.InputError("profile_not_found", "文件中没有该 Profile；用 profile list 查看名称。")
	}
	selection := &selectedProfile{
		Name:       name,
		File:       path,
		SHA256:     hash,
		Profile:    p,
		Applied:    []string{},
		Version:    file.Version,
		Definition: &p,
		FileSpec:   file,
	}
	cmd.SetContext(context.WithValue(cmd.Context(), profileContextKey{}, selection))
	if file.Version == 2 {
		if err := selectV2Profile(cmd, selection); err != nil {
			return deps, err
		}
		if profileInspection(cmd) {
			return deps, nil
		}
	} else {
		component, _ := cmd.Flags().GetString("component")
		binding, _ := cmd.Flags().GetString("binding")
		if component != "" || binding != "" {
			return deps, monitorapi.InputError(
				"profile_version_mismatch",
				"--component/--binding 需要 version: 2。",
			)
		}
	}
	return bindProfileRuntime(cmd, deps, selection)
}

func runMonitor(
	deps systemcmd.BuildDeps,
	fn func(*cobra.Command, *monitorapi.Runtime) error,
) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		bound, err := selectProfile(cmd, deps)
		if err == nil {
			if p := currentProfile(
				cmd,
			); p != nil && p.Version == 2 && cmd.Name() == "check" &&
				cmd.Flags().Lookup("tool") == nil {
				return runV2Report(cmd, bound, p, false, false, false)
			}
			raw, resolveErr := systemcmd.ResolveRuntime(bound)
			err = resolveErr
			rt := monitorapi.Bind(cmd, raw)
			if err == nil {
				cmd.SetContext(context.WithValue(cmd.Context(), runtimeContextKey{}, rt.ContextName))
				err = fn(cmd, rt)
			}
		}
		return printMonitorFailure(cmd, err)
	}
}

func profileMetadata(selection *selectedProfile) map[string]any {
	if selection.Version == 2 {
		meta := map[string]any{
			"version":             2,
			"name":                selection.Name,
			"file":                selection.File,
			"sha256":              selection.SHA256,
			"component":           selection.Component,
			"binding":             selection.Binding,
			"source":              selection.Source,
			"applied_fields":      selection.Applied,
			"ignored_fields":      selection.Ignored,
			"resolved_parameters": selection.Resolved,
		}
		if selection.Route != nil {
			meta["module"] = selection.Route.Module
			meta["context"] = selection.Profile.Context
			meta["bk_biz_id"] = selection.Profile.BizID
		}
		return meta
	}
	return map[string]any{
		"name": selection.Name, "file": selection.File, "sha256": selection.SHA256,
		"context": selection.Profile.Context, "bk_biz_id": selection.Profile.BizID,
		"applied_fields": selection.Applied, "resolved_parameters": selection.Resolved,
	}
}

// Preserve data's original shape for scripts and --from. The profile field is an
// opt-in, monitor-local addition, not a change to the shared Envelope contract.
func writeMonitorEnvelope(cmd *cobra.Command, env *output.Envelope) error {
	selection := currentProfile(cmd)
	if selection == nil {
		return output.WriteCommandEnvelope(cmd, env)
	}
	return output.WriteCommandEnvelopeWithMetadata(cmd, env, map[string]any{"profile": profileMetadata(selection)})
}

func printMonitorFailure(cmd *cobra.Command, err error) error {
	selection := currentProfile(cmd)
	var decision *monitorapi.Decision
	if selection != nil && errors.As(err, &decision) {
		for i, next := range decision.Next {
			if strings.HasPrefix(next, "bk-cli monitor ") {
				decision.Next[i] = next + profileSuffix(selection)
			}
		}
		return monitorapi.PrintFailure(cmd, err, map[string]any{"profile": profileMetadata(selection)})
	}
	return monitorapi.PrintFailure(cmd, err)
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
