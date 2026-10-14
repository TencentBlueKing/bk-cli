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

// Package plugin provides the `bk-cli plugin` management commands.
package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	pluginlib "github.com/TencentBlueKing/bk-cli/internal/plugin"
)

// ReportError writes err as a JSON envelope to w. exitCode 0 keeps the error's own category.
// A plugin ExitStatus is returned untouched because the child already produced its output.
func ReportError(w io.Writer, err error, exitCode int) error {
	var status *pluginlib.ExitStatus
	if errors.As(err, &status) {
		return err
	}
	var pErr *pluginlib.Error
	if !errors.As(err, &pErr) {
		pErr = pluginlib.SystemError(pluginlib.CodeIOError, err.Error(), "")
	}
	code := pErr.ExitCode
	if exitCode != 0 {
		code = exitCode
	}
	_ = output.Err(pErr.Code, pErr.Message, pErr.Hint).WriteJSON(w)
	return &output.CLIError{ExitCode: code, Code: pErr.Code, Message: pErr.Message}
}

// NewPluginCmd builds `bk-cli plugin` and its subcommands.
func NewPluginCmd(manager *pluginlib.Manager, isDryRun, isInsecure func() bool) *cobra.Command {
	root := &cobra.Command{
		Use:   "plugin",
		Short: "Manage approved third-party CLI plugins",
		Long: `Manage third-party CLIs approved by the catalog built into this bk-cli release.

Examples:
  bk-cli plugin list
  bk-cli plugin install bkms
  bk-cli plugin install bkms --version v1.0.4
  bk-cli plugin install bkms --version v1.0.4 --from-file ./bkms-cli_1.0.4_linux_amd64.tar.gz
  bk-cli plugin update bkms
  bk-cli plugin remove bkms
  bk-cli plugin install bkms --dry-run
  bk-cli plugin update bkms --dry-run
  bk-cli plugin remove bkms --dry-run`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	fail := func(cmd *cobra.Command, err error) error { return ReportError(cmd.ErrOrStderr(), err, 0) }
	rejectInsecure := func() error {
		if isInsecure() {
			return pluginlib.UserError(
				pluginlib.CodeUnsupportedHostFlag,
				"--insecure is not supported for plugin downloads",
				"Remove --insecure; plugin downloads always verify TLS",
			)
		}
		return nil
	}
	preview := func(cmd *cobra.Command, operation, name string, r pluginlib.Resolved, source string) error {
		env := &output.Envelope{OK: true, DryRun: true, Data: map[string]any{
			"operation": operation,
			"plugin":    name,
			"version":   r.Version,
			"platform":  r.Platform,
			"source":    source,
		}}
		return env.WriteJSON(cmd.OutOrStdout())
	}
	cmdCtx := func(cmd *cobra.Command) context.Context {
		if ctx := cmd.Context(); ctx != nil {
			return ctx
		}
		return context.Background()
	}
	managementContext := func(cmd *cobra.Command) (context.Context, context.CancelFunc) {
		return signal.NotifyContext(cmdCtx(cmd), os.Interrupt, syscall.SIGTERM)
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List catalog plugins and installed versions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			installed, err := manager.Installed()
			if err != nil {
				return fail(cmd, err)
			}
			return output.SuccessData(
				map[string]any{"plugins": listRows(manager, installed)},
			).WriteJSON(
				cmd.OutOrStdout(),
			)
		},
	}

	var version, fromFile string
	install := &cobra.Command{
		Use:   "install <name>",
		Short: "Install an approved plugin version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := managementContext(cmd)
			defer stop()
			if err := rejectInsecure(); err != nil {
				return fail(cmd, err)
			}
			r, err := manager.Catalog.Resolve(args[0], version, manager.Platform)
			if err != nil {
				return fail(cmd, err)
			}
			if isDryRun() {
				source := r.Asset.URL
				if fromFile != "" {
					source = fromFile
				}
				return preview(cmd, "install", args[0], r, source)
			}
			res, err := manager.Install(
				ctx,
				args[0],
				pluginlib.InstallOptions{Version: version, FromFile: fromFile},
			)
			if err != nil {
				return fail(cmd, err)
			}
			return output.SuccessData(res).WriteJSON(cmd.OutOrStdout())
		},
	}
	install.Flags().StringVar(&version, "version", "", "Exact approved version (default: catalog recommended)")
	install.Flags().StringVar(&fromFile, "from-file", "", "Install from a local official release archive (offline)")

	update := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an installed plugin to the catalog recommended version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := managementContext(cmd)
			defer stop()
			if err := rejectInsecure(); err != nil {
				return fail(cmd, err)
			}
			if isDryRun() {
				current, err := manager.InstalledVersion(args[0])
				if err != nil {
					return fail(cmd, err)
				}
				if current == "" {
					return fail(cmd, pluginlib.UserError(
						pluginlib.CodeNotInstalled,
						fmt.Sprintf(
							"plugin %q is not installed",
							args[0],
						),
						"Run: bk-cli plugin install "+args[0],
					))
				}
				r, err := manager.Catalog.Resolve(args[0], "", manager.Platform)
				if err != nil {
					return fail(cmd, err)
				}
				return preview(cmd, "update", args[0], r, r.Asset.URL)
			}
			res, err := manager.Update(ctx, args[0])
			if err != nil {
				return fail(cmd, err)
			}
			return output.SuccessData(res).WriteJSON(cmd.OutOrStdout())
		},
	}

	remove := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, stop := managementContext(cmd)
			defer stop()
			if isDryRun() {
				current, err := manager.InstalledVersion(args[0])
				if err != nil {
					return fail(cmd, err)
				}
				env := &output.Envelope{OK: true, DryRun: true, Data: map[string]any{
					"operation": "remove",
					"plugin":    args[0],
					"version":   current,
					"platform":  manager.Platform,
				}}
				return env.WriteJSON(cmd.OutOrStdout())
			}
			removed, err := manager.Remove(args[0])
			if err != nil {
				return fail(cmd, err)
			}
			return output.SuccessData(
				map[string]any{"plugin": args[0], "removed": removed},
			).WriteJSON(
				cmd.OutOrStdout(),
			)
		},
	}

	root.AddCommand(list, install, update, remove)
	return root
}

func listRows(manager *pluginlib.Manager, installed map[string]string) []map[string]any {
	names := manager.Catalog.Names()
	for name := range installed {
		if _, ok := manager.Catalog.Plugins[name]; !ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	rows := make([]map[string]any, 0, len(names))
	for _, name := range names {
		def := manager.Catalog.Plugins[name]
		approved := []string{}
		for v, rel := range def.Versions {
			if _, ok := rel.Platforms[manager.Platform]; ok && rel.Status == "allowed" {
				approved = append(approved, v)
			}
		}
		sort.Slice(approved, func(i, j int) bool { return semver.Compare(approved[i], approved[j]) < 0 })

		recAuth := ""
		if rec, ok := def.Versions[def.RecommendedVersion]; ok {
			recAuth = rec.Auth
		}
		status, auth := "not_installed", recAuth
		if v := installed[name]; v != "" {
			status = "not_allowed"
			if rel, ok := def.Versions[v]; ok {
				auth = rel.Auth
			} else {
				auth = ""
			}
			if _, err := manager.Catalog.Resolve(name, v, manager.Platform); err == nil {
				status = "allowed"
			}
		}
		rows = append(rows, map[string]any{
			"name": name, "description": def.Description, "recommended_version": def.RecommendedVersion,
			"approved_versions": approved, "installed_version": installed[name],
			"installed_status": status, "auth": auth,
		})
	}
	return rows
}
