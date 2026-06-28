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

// Package job contains reusable Job-domain operations shared by CLI surfaces.
package job

import (
	"encoding/base64"
	"fmt"
	"maps"
	"os"
	"strconv"
	"strings"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

const GatewayName = "bk-job"

var validScriptLanguages = map[string]int{
	"shell":      1,
	"bat":        2,
	"perl":       3,
	"python":     4,
	"powershell": 5,
}

type FastExecuteScriptInput struct {
	BizID            int
	ScriptContent    string
	ScriptFile       string
	ScriptLanguage   string
	TargetServer     map[string]any
	TargetServerJSON string
	AccountAlias     string
	ScriptParam      string
	Timeout          int
	TaskName         string
	Stage            string
	BodyOverride     string
	Headers          []string
}

// ValidScriptLanguages returns the supported Job script language names and IDs.
func ValidScriptLanguages() map[string]int {
	return maps.Clone(validScriptLanguages)
}

// BuildFastExecuteScriptRequest builds the shared request spec for fast_execute_script.
func BuildFastExecuteScriptRequest(input FastExecuteScriptInput) (syslib.RequestSpec, error) {
	bodyJSON, err := BuildFastExecuteScriptBody(input)
	if err != nil {
		return syslib.RequestSpec{}, err
	}

	return syslib.RequestSpec{
		GatewayName: GatewayName,
		Method:      "POST",
		Path:        "/api/v3/fast_execute_script",
		BodyJSON:    bodyJSON,
		Headers:     input.Headers,
		Stage:       input.Stage,
		AuthConfig: &syslib.AuthConfig{
			AppVerifiedRequired:        true,
			UserVerifiedRequired:       true,
			ResourcePermissionRequired: true,
		},
	}, nil
}

// FastExecuteScript executes the shared Job fast_execute_script operation.
func FastExecuteScript(runtime *syslib.Runtime, input FastExecuteScriptInput) (*syslib.RequestResult, error) {
	spec, err := BuildFastExecuteScriptRequest(input)
	if err != nil {
		return nil, err
	}

	return syslib.ExecuteRequest(runtime, spec)
}

// BuildFastExecuteScriptBody synthesizes or forwards the request body for fast_execute_script.
func BuildFastExecuteScriptBody(input FastExecuteScriptInput) (string, error) {
	if input.BodyOverride != "" {
		return input.BodyOverride, nil
	}
	if err := validateBizID(input.BizID); err != nil {
		return "", err
	}

	resolvedScriptContent, err := resolveScriptContent(input.ScriptContent, input.ScriptFile)
	if err != nil {
		return "", err
	}

	targetServer, err := resolveTargetServer(input.TargetServer, input.TargetServerJSON)
	if err != nil {
		return "", err
	}

	langID, ok := validScriptLanguages[input.ScriptLanguage]
	if !ok {
		return "", output.UserError(
			"invalid_argument",
			"script_language must be one of: shell, bat, perl, python, powershell",
			"Use --script_language shell",
		)
	}

	payload := buildBizScopePayload(input.BizID)
	maps.Copy(payload, map[string]any{
		"script_language": langID,
		"script_content":  base64.StdEncoding.EncodeToString([]byte(resolvedScriptContent)),
		"target_server":   targetServer,
		"timeout":         input.Timeout,
	})
	if input.AccountAlias != "" {
		payload["account_alias"] = input.AccountAlias
	}
	if input.ScriptParam != "" {
		payload["script_param"] = base64.StdEncoding.EncodeToString([]byte(input.ScriptParam))
	}
	if input.TaskName != "" {
		payload["task_name"] = input.TaskName
	}

	return systemcmd.MarshalJSON(payload)
}

func buildBizScopePayload(bizID int) map[string]any {
	return map[string]any{
		"bk_scope_type": "biz",
		"bk_scope_id":   strconv.Itoa(bizID),
		"bk_biz_id":     bizID,
	}
}

func validateBizID(bizID int) error {
	return systemcmd.ValidatePositiveIntFlag("bk_biz_id", bizID)
}

func resolveTargetServer(targetServer map[string]any, targetServerJSON string) (map[string]any, error) {
	if targetServer != nil {
		return targetServer, nil
	}

	return systemcmd.ParseJSONObjectFlag("target_server", targetServerJSON)
}

func resolveScriptContent(scriptContent, scriptFile string) (string, error) {
	contentProvided := strings.TrimSpace(scriptContent) != ""
	fileProvided := strings.TrimSpace(scriptFile) != ""

	if contentProvided && fileProvided {
		return "", output.UserError(
			"invalid_argument",
			"script_content and script_file cannot be used together",
			"Use exactly one of --script_content or --script_file",
		)
	}
	if !contentProvided && !fileProvided {
		return "", output.UserError(
			"invalid_argument",
			"one of script_content or script_file is required when --body is not provided",
			"Use --script_content 'echo hello', --script_file ./script.sh, or provide an explicit --body",
		)
	}
	if contentProvided {
		if err := systemcmd.ValidateNonEmptyStringFlag("script_content", scriptContent); err != nil {
			return "", err
		}
		return scriptContent, nil
	}
	if err := systemcmd.ValidateNonEmptyStringFlag("script_file", scriptFile); err != nil {
		return "", err
	}

	scriptBytes, err := os.ReadFile(scriptFile)
	if err != nil {
		return "", output.UserError(
			"invalid_argument",
			fmt.Sprintf("failed to read script_file: %v", err),
			"Use --script_file with a readable local file path",
		)
	}
	if err := systemcmd.ValidateNonEmptyStringFlag("script_file content", string(scriptBytes)); err != nil {
		return "", err
	}

	return string(scriptBytes), nil
}
