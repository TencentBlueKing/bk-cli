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
	"strconv"

	json "github.com/goccy/go-json"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
)

// Carry only permission resource identifiers, never query text, time windows or log bodies.
func permissionHint(contextName, action, name, biz string, values map[string]any) string {
	bizArg := biz
	if _, err := strconv.ParseInt(biz, 10, 64); err != nil {
		bizArg = shellQuote(biz)
	}
	command := "bk-cli monitor permission " + action + " --bk_biz_id " + bizArg + " --tool " + name
	resource := map[string]any{}
	for _, key := range []string{"index_set_id", "target_type", "app_name", "service_name", "dashboard_uid"} {
		if value, exists := values[key]; exists {
			switch value.(type) {
			case string, int, int64, json.Number:
				resource[key] = value
			}
		}
	}
	if len(resource) > 0 {
		body, err := json.Marshal(resource)
		if err == nil {
			command += " --body " + shellQuote(string(body))
		}
	}
	if contextName != "" {
		command += " --context " + contextName
	}
	return command
}

func preservePermissionTarget(err error, contextName, name, biz string, values map[string]any) error {
	var decision *monitorapi.Decision
	if errors.As(err, &decision) &&
		(decision.Code == "permission_missing" || decision.Code == "permission_denied") {
		decision.Next = []string{permissionHint(contextName, "apply", name, biz, values)}
	}
	return err
}

func profileSuffix(p *selectedProfile) string {
	if p == nil {
		return ""
	}
	suffix := " --profile " + p.Name + " --profile_file " + shellQuote(p.File)
	if p.Version == 2 {
		if p.Component != "" {
			suffix += " --component " + p.Component
		}
		if p.Binding != "" {
			suffix += " --binding " + p.Binding
		}
	}
	return suffix
}

func profileDiagnosticNext(row map[string]any, name string, p *selectedProfile) []string {
	code, _ := row["decision"].(string)
	suffix := profileSuffix(p)
	switch code {
	case "permission_missing", "permission_denied":
		return []string{permissionHint(p.Profile.Context, "apply", name, p.Profile.BizID, nil) + suffix}
	case "auth_failed", "auth_required", "identity_unverified":
		return []string{"bk-cli auth status --context " + p.Profile.Context}
	case "resource_not_found", "resource_not_visible", "needs_resource":
		commands := nextCommands(name, p.Profile.BizID)
		if len(commands) > 0 {
			for i := range commands {
				commands[i] += suffix
			}
			return commands
		}
	}
	// Metadata/service failures are not fixed by applying for more permissions.
	return []string{"bk-cli monitor profile show" + suffix}
}
