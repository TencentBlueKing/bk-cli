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

func newTransferHostToRecycleModuleCmd(deps systemcmd.BuildDeps) *cobra.Command {
	return newTransferHostsToSingleTargetCmd(transferHostsToSingleTargetSpec{
		Name:    "transfer_host_to_recycle_module",
		Short:   "Transfer hosts to the recycle module",
		Path:    "/api/v3/open/hosts/modules/recycle",
		Example: "  bk-cli cmdb transfer_host_to_recycle_module --bk_biz_id 2 --bk_host_ids 1,2",
	}, deps)
}
