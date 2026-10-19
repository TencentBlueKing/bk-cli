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

package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	plugincmd "github.com/TencentBlueKing/bk-cli/cmd/plugin"
	pluginlib "github.com/TencentBlueKing/bk-cli/internal/plugin"
)

const (
	pluginAnnotation    = "bk-cli/plugin"
	pluginCommandPrefix = "[plugin] "
)

var rootPluginManager *pluginlib.Manager

type pluginInvocation struct {
	Name    string
	Context string
	Args    []string
	BadFlag string
}

var unsupportedHostFlags = map[string]bool{
	"--dry-run":  true,
	"-v":         true,
	"--verbose":  true,
	"--insecure": true,
}

// splitPluginInvocation inspects only the host prefix before the first positional argument.
func splitPluginInvocation(args []string, names map[string]bool) (pluginInvocation, bool) {
	var call pluginInvocation
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, _, _ := strings.Cut(arg, "=")
		switch {
		case arg == "--context":
			if i+1 >= len(args) {
				return pluginInvocation{}, false
			}
			call.Context = args[i+1]
			i++
		case strings.HasPrefix(arg, "--context="):
			call.Context = strings.TrimPrefix(arg, "--context=")
		case arg == "-h" || arg == "--help":
			// Host help before a plugin name remains Cobra root help.
			return pluginInvocation{}, false
		case unsupportedHostFlags[name]:
			if call.BadFlag == "" {
				call.BadFlag = name
			}
		case strings.HasPrefix(arg, "-"):
			return pluginInvocation{}, false
		default:
			if !names[arg] {
				return pluginInvocation{}, false
			}
			call.Name = arg
			call.Args = append([]string{}, args[i+1:]...)
			return call, true
		}
	}
	return pluginInvocation{}, false
}

func pluginNames(root *cobra.Command) map[string]bool {
	names := map[string]bool{}
	if root == nil {
		return names
	}
	for _, command := range root.Commands() {
		if command.Annotations[pluginAnnotation] == "true" {
			names[command.Name()] = true
		}
	}
	return names
}

// attachPluginCommands adds help-only stubs; execution is dispatched in executeRoot.
func attachPluginCommands(root *cobra.Command, manager *pluginlib.Manager) []string {
	reserved := map[string]bool{
		"help": true, "completion": true, "plugin": true, "__complete": true, "__completeNoDesc": true,
	}
	for _, command := range root.Commands() {
		reserved[command.Name()] = true
		for _, alias := range command.Aliases {
			reserved[alias] = true
		}
	}
	installed, installedErr := manager.Installed()
	var skipped []string
	for _, name := range manager.Catalog.Names() {
		if reserved[name] {
			skipped = append(skipped, name)
			continue
		}
		definition := manager.Catalog.Plugins[name]
		status := "not installed"
		if installedErr != nil {
			status = "install state unreadable"
		} else if version := installed[name]; version != "" {
			status = "installed " + version
		}
		root.AddCommand(&cobra.Command{
			Use:   name + " [args...]",
			Short: fmt.Sprintf("%s%s (%s)", pluginCommandPrefix, definition.Description, status),
			Long: fmt.Sprintf(`%s

Third-party CLI %q approved by the bk-cli catalog. Status: %s.
All arguments after %q are passed to the plugin unchanged.

Examples:
  bk-cli plugin install %s
  bk-cli %s --help
  bk-cli --context NAME %s ...`,
				definition.Description,
				definition.Binary,
				status,
				name,
				name,
				name,
				name,
			),
			Annotations:        map[string]string{pluginAnnotation: "true"},
			DisableFlagParsing: true,
			Args:               cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return plugincmd.ReportError(
					cmd.ErrOrStderr(),
					pluginlib.UserError(
						pluginlib.CodeUnsupportedHostFlag,
						fmt.Sprintf("unsupported host flag before plugin %q", name),
						fmt.Sprintf(
							"Only --context is allowed before the plugin name; put plugin flags after %q",
							name,
						),
					),
					pluginlib.ExitHostFailure,
				)
			},
		})
	}
	sort.Strings(skipped)
	return skipped
}

func runPlugin(root *cobra.Command, manager *pluginlib.Manager, call pluginInvocation) error {
	if call.BadFlag != "" {
		return plugincmd.ReportError(
			root.ErrOrStderr(),
			pluginlib.UserError(
				pluginlib.CodeUnsupportedHostFlag,
				fmt.Sprintf("%s is not supported before plugin %q", call.BadFlag, call.Name),
				fmt.Sprintf(
					"Only --context is allowed before the plugin name; put plugin flags after %q",
					call.Name,
				),
			),
			pluginlib.ExitHostFailure,
		)
	}
	err := manager.Run(call.Name, call.Context, call.Args, pluginlib.Streams{
		In: root.InOrStdin(), Out: root.OutOrStdout(), Err: root.ErrOrStderr(),
	})
	if err == nil {
		return nil
	}
	return plugincmd.ReportError(root.ErrOrStderr(), err, pluginlib.ExitHostFailure)
}
