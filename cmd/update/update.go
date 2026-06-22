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

// Package update defines the Cobra command for self-updating bk-cli.
package update

import (
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	"github.com/TencentBlueKing/bk-cli/internal/update"
)

const (
	defaultGitHubOwner = "TencentBlueKing"
	defaultGitHubRepo  = "bk-cli"
)

// NewUpdateCmd creates the update subcommand.
func NewUpdateCmd(getVersion func() string, isDryRun func() bool) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update bk-cli to the latest version",
		Long: `Check for a newer version and update in place.

If bk-cli was installed via npm, this command will run npm install to update automatically.

Examples:
  # Check and update
  bk-cli update

  # Preview only (don't download)
  bk-cli update --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if update.IsNPMInstall() {
				if isDryRun() {
					data := map[string]any{
						"install_method": "npm",
						"updated":        false,
						"dry_run":        true,
					}
					return output.SuccessData(data).WriteJSON(cmd.OutOrStdout())
				}

				out, err := update.RunNPMUpdate(cmd.Context())
				if err != nil {
					return output.SystemError(
						"npm_update_failed",
						err.Error(),
						"Run manually: npm i -g @blueking/bk-cli",
					)
				}

				data := map[string]any{
					"install_method": "npm",
					"updated":        true,
					"npm_output":     string(out),
				}
				return output.SuccessData(data).WriteJSON(cmd.OutOrStdout())
			}

			current := getVersion()

			source := &update.GitHubSource{Owner: defaultGitHubOwner, Repo: defaultGitHubRepo}
			release, err := source.CheckLatest(cmd.Context())
			if err != nil {
				return output.SystemError("update_check_failed", err.Error(),
					"Check network connectivity")
			}

			latest := release.Version

			if !update.IsNewer(current, latest) {
				data := map[string]any{
					"current_version": current,
					"latest_version":  latest,
					"updated":         false,
				}
				return output.SuccessData(data).WriteJSON(cmd.OutOrStdout())
			}

			if isDryRun() {
				data := map[string]any{
					"current_version": current,
					"latest_version":  latest,
					"updated":         false,
					"dry_run":         true,
				}
				return output.SuccessData(data).WriteJSON(cmd.OutOrStdout())
			}

			if err := update.DownloadAndReplace(release.DownloadURL); err != nil {
				return output.SystemError("update_failed", err.Error(), "")
			}

			data := map[string]any{
				"current_version": current,
				"latest_version":  latest,
				"updated":         true,
			}
			return output.SuccessData(data).WriteJSON(cmd.OutOrStdout())
		},
	}
}
