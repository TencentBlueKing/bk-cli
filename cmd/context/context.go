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

// Package context defines Cobra commands for managing CLI contexts.
package context

import "github.com/spf13/cobra"

// NewContextCmd creates the parent "context" command.
func NewContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage CLI contexts (BlueKing deployments)",
		Long: `Manage named contexts, each targeting a different BlueKing deployment.

Each context has its own config (URL template, tenant) and credentials. And the data is stored in ~/.bk-cli/contexts.

Examples:
  # First-run setup
  bk-cli context init --bk_api_url_tmpl="https://bkapi.example.com/api/{gateway_name}/"

  # Create a new context (it does not become active automatically)
  bk-cli context create clouds --bk_api_url_tmpl="https://bkapi.clouds.example.com/api/{gateway_name}/"

  # List all contexts
  bk-cli context list

  # Show the active context configuration
  bk-cli context status

  # Switch active context
  bk-cli context use clouds

  # Delete a context
  bk-cli context delete old-env`,
	}
	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newUseCmd())
	cmd.AddCommand(newDeleteCmd())
	return cmd
}
