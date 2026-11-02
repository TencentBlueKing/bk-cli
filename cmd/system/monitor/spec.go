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

// Package monitor implements user-mode BlueKing Monitor commands backed by individual HTTP APIs.
package monitor

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

var catalogData, toolsByName, catalogErr = loadCatalog()

func init() {
	if catalogErr == nil {
		monitorapi.SetCatalog(supportedTools(), commandName, nextCommands)
		routes := map[string]monitorapi.Endpoint{}
		for _, item := range catalogData.Tools {
			routes[item.Name] = item.Endpoint
		}
		monitorapi.SetEndpoints(routes)
	}
}

// NewSystemSpec returns the monitor system spec grouped by observability module.
func NewSystemSpec() systemcmd.SystemSpec {
	if catalogErr != nil {
		return systemcmd.SystemSpec{
			Name: "monitor", Description: "BlueKing Monitor user-mode observability commands",
			RegisterGoActions: func(*cobra.Command, systemcmd.BuildDeps) error { return catalogErr },
		}
	}
	subsystems := []systemcmd.SystemSpec{newToolsSpec(toolsByName), newPermissionSpec(), newProfileSpec()}
	for _, item := range modules {
		current := item
		subsystems = append(subsystems, systemcmd.SystemSpec{
			Name:        current.Name,
			Description: current.Description,
			RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
				parent.Long = current.Description + ".\n\n" + modulePermissionHelp(current.Name)
				for _, item := range catalogData.Tools {
					if item.Module == current.Name {
						parent.AddCommand(newToolCmd(item, deps))
					}
				}
				return nil
			},
		})
	}
	return systemcmd.SystemSpec{
		Name:        "monitor",
		Description: "BlueKing Monitor user-mode observability commands",
		RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
			parent.AddCommand(newStatusCmd(deps))
			return nil
		},
		Subsystems: subsystems,
	}
}

func supportedTools() []string {
	names := make([]string, 0, len(catalogData.Tools))
	for _, item := range catalogData.Tools {
		names = append(names, item.Name)
	}
	return names
}

func commandName(name string) string {
	item, ok := toolsByName[name]
	if !ok {
		return ""
	}
	return fmt.Sprintf("bk-cli monitor %s %s", item.Module, item.Name)
}

func nextCommands(name, biz string) []string {
	item, ok := toolsByName[name]
	if !ok {
		return nil
	}
	var commands []string
	for _, previous := range item.Prerequisites {
		if command := commandName(previous); command != "" {
			if biz != "" && biz != "<nil>" && hasParam(toolsByName[previous], "bk_biz_id") {
				command += " --bk_biz_id " + biz
			}
			commands = append(commands, command)
		}
	}
	return commands
}

func printReply(cmd *cobra.Command, reply *monitorapi.Reply, err error) error {
	if err != nil {
		return err
	}
	return writeMonitorEnvelope(cmd, reply.Envelope(reply.Data))
}
