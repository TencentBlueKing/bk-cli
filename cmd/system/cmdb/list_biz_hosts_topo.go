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

package cmdb

import (
	"fmt"

	"github.com/spf13/cobra"

	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newListBizHostsTopoCmd(deps systemcmd.BuildDeps) *cobra.Command {
	const defaultLimit = 500

	return newHostListCommand(hostListCommandSpec{
		Name:          "list_biz_hosts_topo",
		Short:         "List business hosts together with topology data",
		Example:       "  bk-cli cmdb list_biz_hosts_topo --bk_biz_id 2 --host_ips 10.0.0.1",
		RequiresBizID: true,
		Fields: []string{
			"bk_host_id",
			"bk_host_innerip",
			"bk_cloud_id",
		},
		Limit: defaultLimit,
		PathBuilder: func(bizID int) string {
			return fmt.Sprintf("/api/v3/open/hosts/app/%d/list_hosts_topo", bizID)
		},
	}, deps)
}
