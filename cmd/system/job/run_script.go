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

	shortcutjob "github.com/TencentBlueKing/bk-cli/internal/shortcut/job"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newRunScriptCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		bizID          int
		hosts          string
		scriptContent  string
		scriptFile     string
		scriptLanguage string
		accountAlias   string
		scriptParam    string
		timeout        int
		taskName       string
		stage          string
		headers        []string
	)

	cmd := &cobra.Command{
		Use:   "+run-script",
		Short: "Resolve hosts from CMDB and run a script through BK-JOB",
		Long: `Resolve host IPs from CMDB, then dispatch a BK-JOB fast script execution task.

This shortcut composes CMDB host lookup with Job fast_execute_script.
It does not wait for completion or fetch execution logs.`,
		Example: strings.Join([]string{
			"  bk-cli job +run-script \\",
			"    --biz 2 --hosts 10.0.0.1,27:10.0.0.2 \\",
			`    --script_content "echo hello" --script_language shell --account_alias root`,
			"  bk-cli job +run-script \\",
			"    --biz 2 --hosts 10.0.0.1 --script_file ./script.sh",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			result, err := shortcutjob.RunScript(runtime, shortcutjob.RunScriptInput{
				BizID:          bizID,
				Hosts:          hosts,
				ScriptContent:  scriptContent,
				ScriptFile:     scriptFile,
				ScriptLanguage: scriptLanguage,
				AccountAlias:   accountAlias,
				ScriptParam:    scriptParam,
				Timeout:        timeout,
				TaskName:       taskName,
				Stage:          stage,
				Headers:        headers,
			})
			if err != nil {
				return err
			}

			if err := systemcmd.EnsureEnvelope("+run-script", result.Envelope); err != nil {
				return err
			}
			return result.Envelope.WriteJSON(cmd.OutOrStdout())
		},
	}

	cmd.Flags().IntVar(&bizID, "biz", 0, "Business ID")
	cmd.Flags().StringVar(&hosts, "hosts", "", "Comma-separated host IPs; use cloud:ip for non-zero cloud IDs")
	cmd.Flags().StringVar(&scriptContent, "script_content", "", "Script content (will be Base64-encoded)")
	cmd.Flags().StringVar(&scriptFile, "script_file", "", "Local script file path")
	cmd.Flags().StringVar(
		&scriptLanguage,
		"script_language",
		"shell",
		"Script language: shell, bat, perl, python, powershell",
	)
	cmd.Flags().StringVar(&accountAlias, "account_alias", "", "Execution account alias (e.g. root)")
	cmd.Flags().StringVar(&scriptParam, "script_param", "", "Script parameters (will be Base64-encoded)")
	cmd.Flags().IntVar(&timeout, "timeout", 7200, "Timeout in seconds")
	cmd.Flags().StringVar(&taskName, "task_name", "", "Task name (optional)")
	systemcmd.AddCommonRequestFlagsWithoutBody(cmd, &stage, &headers)
	cmd.MarkFlagsMutuallyExclusive("script_content", "script_file")

	return cmd
}
