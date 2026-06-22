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

package context

import (
	"fmt"
	"maps"
	"strings"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

func contextConfigData(cfg *config.Config) (map[string]any, error) {
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal context config: %w", err)
	}

	var cfgData map[string]any
	if err := yaml.Unmarshal(raw, &cfgData); err != nil {
		return nil, fmt.Errorf("failed to convert context config: %w", err)
	}

	data := make(map[string]any, len(cfgData))
	maps.Copy(data, cfgData)
	return data, nil
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all contexts",
		Long: `List all configured contexts and show which one is active.

Examples:
  bk-cli context list`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			names, err := config.ListContexts()
			if err != nil {
				return output.UserError("list_contexts_failed", err.Error(), "")
			}

			active, err := config.ActiveContextName()
			if err != nil {
				return output.UserError("read_active_failed", err.Error(), "")
			}

			contexts := make([]map[string]any, 0, len(names))
			failures := make([]string, 0)
			if active != "" {
				if err := validate.ValidateContextName(active); err != nil {
					failures = append(failures, fmt.Sprintf("%s (%s)", active, err))
				}
			}
			for _, name := range names {
				if err := validate.ValidateContextName(name); err != nil {
					failures = append(failures, fmt.Sprintf("%s (%s)", name, err))
					continue
				}
				cfg, loadErr := config.Load(config.ConfigPath(name))
				if loadErr != nil {
					failures = append(failures, fmt.Sprintf("%s (%s)", name, loadErr))
					continue
				}
				contextData, dataErr := contextConfigData(cfg)
				if dataErr != nil {
					failures = append(failures, fmt.Sprintf("%s (%s)", name, dataErr))
					continue
				}
				contextData["name"] = name
				contexts = append(contexts, contextData)
			}
			if len(failures) > 0 {
				return output.UserError(
					"invalid_context_config",
					"failed contexts: "+strings.Join(failures, "; "),
					"Fix or remove the broken context directories, then rerun `bk-cli context list`",
				)
			}

			data := map[string]any{
				"active":   active,
				"contexts": contexts,
			}

			return output.SuccessData(data).WriteJSON(cmd.OutOrStdout())
		},
	}
}
