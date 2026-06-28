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
	"strings"

	"github.com/spf13/cobra"

	jobsvc "github.com/TencentBlueKing/bk-cli/internal/job"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

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

			result, err := jobsvc.FastExecuteScript(runtime, jobsvc.FastExecuteScriptInput{
				BizID:            bizID,
				ScriptContent:    scriptContent,
				ScriptFile:       scriptFile,
				ScriptLanguage:   scriptLanguage,
				TargetServerJSON: targetServer,
				AccountAlias:     accountAlias,
				ScriptParam:      scriptParam,
				Timeout:          timeout,
				TaskName:         taskName,
				Stage:            stage,
				BodyOverride:     body,
				Headers:          headers,
			})
			if err != nil {
				return err
			}

			if err := systemcmd.EnsureEnvelope("fast_execute_script", result.Envelope); err != nil {
				return err
			}
			return result.Envelope.WriteJSON(cmd.OutOrStdout())
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
