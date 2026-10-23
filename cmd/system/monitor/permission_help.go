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
	"fmt"
	"sort"
	"strings"
)

// permissionLines describes the reviewed metadata in help text. The live decision always comes from the server.
func permissionLines(item tool) []string {
	spec := item.Permission
	if spec.Mode == "exempt" {
		return []string{"Permission: platform-visible metadata; no business permission is required."}
	}
	lines := []string{"Permission (any one is enough):"}
	if spec.MCP != nil {
		lines = append(lines, "  1. "+describeAction(*spec.MCP)+" on "+resourceLabel(spec.MCP.ResourceType))
	}
	if spec.Native != nil {
		line := "  2. " + describeAction(
			spec.Native.iamAction,
		) + " on " + resourceLabel(
			spec.Native.ResourceType,
		)
		if spec.Native.Conditional != nil {
			c := spec.Native.Conditional
			line += fmt.Sprintf(
				"; when %s: %s on %s",
				c.When,
				describeAction(c.iamAction),
				resourceLabel(c.ResourceType),
			)
		}
		lines = append(lines, line)
	}
	lines = append(lines,
		"Check:  bk-cli monitor permission check --bk_biz_id <bk_biz_id> --tool "+item.Name,
		"Apply:  bk-cli monitor permission apply --bk_biz_id <bk_biz_id> --tool "+item.Name,
	)
	return lines
}

func describeAction(action iamAction) string {
	key := action.SystemID + ":" + action.ActionID
	if name := catalogData.Actions[key]; name != "" {
		return fmt.Sprintf("%s (%s)", name, key)
	}
	return key
}

func resourceLabel(kind string) string {
	if label := catalogData.ResourceTypes[kind]; label != "" {
		return label
	}
	return kind
}

// moduleActions returns the distinct actions used by a module, for module-level permission guidance.
func moduleActions(moduleName string) []string {
	seen := map[string]bool{}
	for _, item := range catalogData.Tools {
		if item.Module != moduleName {
			continue
		}
		if item.Permission.MCP != nil {
			seen["MCP: "+describeAction(*item.Permission.MCP)] = true
		}
		if item.Permission.Native != nil {
			seen["Native: "+describeAction(item.Permission.Native.iamAction)] = true
			if c := item.Permission.Native.Conditional; c != nil {
				seen["Native: "+describeAction(c.iamAction)] = true
			}
		}
	}
	values := make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func modulePermissionHelp(moduleName string) string {
	actions := moduleActions(moduleName)
	if len(actions) == 0 {
		return "Permission: platform-visible metadata; no business permission is required."
	}
	return strings.Join([]string{
		"Permission: MCP permission is checked first; native SaaS permission is checked only after explicit denial.",
		"Actions used by this module:",
		"  " + strings.Join(actions, "\n  "),
		"List:   bk-cli monitor permission list --bk_biz_id <bk_biz_id> --module " + moduleName,
		"Apply:  bk-cli monitor permission apply --bk_biz_id <bk_biz_id> --module " + moduleName,
	}, "\n")
}
