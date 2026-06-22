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
	"github.com/spf13/cobra"

	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newListHostsWithoutBizCmd(deps systemcmd.BuildDeps) *cobra.Command {
	const defaultLimit = 500

	return newHostListCommand(hostListCommandSpec{
		Name:    "list_hosts_without_biz",
		Short:   "List hosts across all businesses",
		Example: "  bk-cli cmdb list_hosts_without_biz --host_ips 10.0.0.1,27:10.0.0.2",
		Fields: []string{
			"bk_host_id",
			"bk_host_innerip",
			"bk_cloud_id",
			"bk_host_name",
			"operator",
			"bk_bak_operator",
		},
		Limit: defaultLimit,
		PathBuilder: func(_ int) string {
			return "/api/v3/open/hosts/list_hosts_without_app"
		},
	}, deps)
}
