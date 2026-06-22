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

package job

import (
	"encoding/base64"
	"fmt"
	"maps"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

var validScriptLanguages = map[string]int{
	"shell":      1,
	"bat":        2,
	"perl":       3,
	"python":     4,
	"powershell": 5,
}

func newFastExecuteScriptCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		bizID          int
		scriptContent  string
		scriptFile     string
		scriptLanguage string
		targetServer   string
		accountAlias   string
		scriptParam    string
		timeout        int
		taskName       string
		stage          string
		body           string
		headers        []string
	)

	cmd := &cobra.Command{
		Use:   "fast_execute_script",
		Short: "Fast execute a script on target hosts",
		Long: `Execute a script on target hosts.

When --body is not provided, bk-cli synthesizes the request body from flag inputs.
Provide exactly one of --script_content or --script_file when synthesizing the request body.
The resolved script content is automatically Base64-encoded before sending.
Use --target_server to provide the target host selector as a JSON object for the synthesized path.
Use --body to pass the full JSON request body directly (content must already be Base64-encoded).
Do not mix partial --body fragments with synthesized flags.`,
		Example: strings.Join([]string{
			"  bk-cli job fast_execute_script \\",
			`    --bk_biz_id 2 --script_content "echo hello" \`,
			"    --script_language shell --account_alias root \\",
			`    --target_server '{"host_id_list":[1]}'`,
			"  bk-cli job fast_execute_script \\",
			"    --bk_biz_id 2 --script_file ./script.sh \\",
			"    --script_language shell --account_alias root \\",
			`    --target_server '{"host_id_list":[1]}'`,
			"  bk-cli job fast_execute_script \\",
			"    --body '{\"bk_biz_id\":2,\"target_server\":{\"host_id_list\":[1]}," +
				"\"script_language\":1,\"script_content\":\"ZWNobyBoZWxsbw==\"}'",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			bodyJSON, err := buildFastExecScriptBody(
				body, bizID, scriptContent, scriptFile, scriptLanguage,
				targetServer, accountAlias, scriptParam, timeout, taskName,
			)
			if err != nil {
				return err
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "fast_execute_script", syslib.RequestSpec{
				GatewayName: gatewayName,
				Method:      "POST",
				Path:        "/api/v3/fast_execute_script",
				BodyJSON:    bodyJSON,
				Headers:     headers,
				Stage:       stage,
				AuthConfig: &syslib.AuthConfig{
					AppVerifiedRequired:        true,
					UserVerifiedRequired:       true,
					ResourcePermissionRequired: true,
				},
			}, nil)
		},
	}

	cmd.Flags().IntVar(&bizID, "bk_biz_id", 0, "Business ID (required when not using --body)")
	cmd.Flags().StringVar(&scriptContent, "script_content", "", "Script content (will be Base64-encoded)")
	cmd.Flags().StringVar(&scriptFile, "script_file", "", "Local script file path")
	cmd.Flags().StringVar(&scriptLanguage, "script_language", "shell",
		"Script language: shell, bat, perl, python, powershell")
	cmd.Flags().StringVar(
		&targetServer,
		"target_server",
		"",
		"Target server selector as a JSON object (required when not using --body)",
	)
	cmd.Flags().StringVar(&accountAlias, "account_alias", "", "Execution account alias (e.g. root)")
	cmd.Flags().StringVar(&scriptParam, "script_param", "", "Script parameters (will be Base64-encoded)")
	cmd.Flags().IntVar(&timeout, "timeout", 7200, "Timeout in seconds")
	cmd.Flags().StringVar(&taskName, "task_name", "", "Task name (optional)")
	systemcmd.AddCommonRequestFlags(cmd, &stage, &body, &headers)
	cmd.MarkFlagsMutuallyExclusive("script_content", "script_file")

	return cmd
}

func buildFastExecScriptBody(
	bodyOverride string,
	bizID int,
	scriptContent, scriptFile, scriptLanguage string,
	targetServer string,
	accountAlias, scriptParam string,
	timeout int,
	taskName string,
) (string, error) {
	if bodyOverride != "" {
		return bodyOverride, nil
	}
	if err := validateBizID(bizID); err != nil {
		return "", err
	}
	resolvedScriptContent, err := resolveFastExecuteScriptContent(scriptContent, scriptFile)
	if err != nil {
		return "", err
	}
	parsedTargetServer, err := systemcmd.ParseJSONObjectFlag("target_server", targetServer)
	if err != nil {
		return "", err
	}

	langID, ok := validScriptLanguages[scriptLanguage]
	if !ok {
		return "", output.UserError(
			"invalid_argument",
			"script_language must be one of: shell, bat, perl, python, powershell",
			"Use --script_language shell",
		)
	}

	payload := buildBizScopePayload(bizID)
	maps.Copy(payload, map[string]any{
		"script_language": langID,
		"script_content":  base64.StdEncoding.EncodeToString([]byte(resolvedScriptContent)),
		"target_server":   parsedTargetServer,
		"timeout":         timeout,
	})

	if accountAlias != "" {
		payload["account_alias"] = accountAlias
	}
	if scriptParam != "" {
		payload["script_param"] = base64.StdEncoding.EncodeToString([]byte(scriptParam))
	}
	if taskName != "" {
		payload["task_name"] = taskName
	}
	return systemcmd.MarshalJSON(payload)
}

func resolveFastExecuteScriptContent(scriptContent, scriptFile string) (string, error) {
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
