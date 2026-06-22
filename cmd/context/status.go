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
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active context configuration",
		Long: `Display configuration for the active (or specified) context.

Examples:
  bk-cli context status
  bk-cli context status --context=clouds`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxOverride, _ := cmd.Root().PersistentFlags().GetString("context")
			ctxName, cfg, err := config.ResolveContextReadOnly(ctxOverride)
			if err != nil {
				return output.UserError("context_error", err.Error(), "Run: bk-cli context list")
			}
			if ctxName == "" {
				return output.SuccessData(map[string]any{
					"context": "(none)",
				}).WriteJSON(cmd.OutOrStdout())
			}

			data, err := contextConfigData(cfg)
			if err != nil {
				return output.UserError("context_error", err.Error(), "Run: bk-cli context list")
			}
			data["context"] = ctxName

			return output.SuccessData(data).WriteJSON(cmd.OutOrStdout())
		},
	}
}
