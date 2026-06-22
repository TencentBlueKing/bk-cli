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

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a context",
		Long: `Delete a named context and its configuration.

The active context cannot be deleted. Switch to another context first.

Examples:
  bk-cli context delete old-env`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validate.ValidateContextName(name); err != nil {
				return output.UserError(
					"invalid_context_name",
					err.Error(),
					"Use a safe context name like default, prod-1, or clouds",
				)
			}

			active, err := config.ActiveContextName()
			if err != nil {
				return output.UserError("read_active_failed", err.Error(), "")
			}

			if name == active {
				return output.UserError("delete_active_context",
					fmt.Sprintf("cannot delete the active context %q", name),
					"Switch to another context first with 'bk-cli context use OTHER-CONTEXT'")
			}

			if err := config.DeleteContext(name); err != nil {
				return output.UserError("delete_context_failed", err.Error(), "")
			}

			return output.Success(fmt.Sprintf("Context %q deleted", name)).WriteJSON(cmd.OutOrStdout())
		},
	}
}
