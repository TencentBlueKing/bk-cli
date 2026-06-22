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

// Package cluster_manager defines the BCS ClusterManager subsystem commands.
package cluster_manager

import systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"

// NewSystemSpec returns the ClusterManager subsystem spec used during command registration.
func NewSystemSpec() systemcmd.SystemSpec {
	return systemcmd.SystemSpec{
		Name:        "cluster_manager",
		Description: "BlueKing Container Service ClusterManager commands",
		YAMLFile:    "bcs/cluster_manager/actions.yaml",
	}
}
