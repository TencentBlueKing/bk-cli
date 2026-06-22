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

package gse

import (
	"github.com/spf13/cobra"

	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
	"github.com/TencentBlueKing/bk-cli/internal/utils"
)

func newListAgentStateCmd(deps systemcmd.BuildDeps) *cobra.Command {
	return newAgentListCommand(agentListCommandSpec{
		Name:  "list_agent_state",
		Short: "Query agent state for a list of agent IDs",
		Example: utils.FormatCommandExamples(
			"bk-cli gse list_agent_state --agent_id_list 02000000000000000000000000000001,02000000000000000000000000000002",
			"bk-cli gse list_agent_state --body '{\"agent_id_list\":[\"02000000000000000000000000000001\"]}'",
		),
		Path: "/api/v2/cluster/list_agent_state",
	}, deps)
}
