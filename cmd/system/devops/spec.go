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

// Package devops implements the DevOps parent command that groups pipeline, CodeCC, and Stream actions.
package devops

import (
	codeccsystem "github.com/TencentBlueKing/bk-cli/cmd/system/devops/codecc"
	pipelinesystem "github.com/TencentBlueKing/bk-cli/cmd/system/devops/pipeline"
	streamsystem "github.com/TencentBlueKing/bk-cli/cmd/system/devops/stream"

	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

// NewSystemSpec returns the top-level devops system spec used during command registration.
func NewSystemSpec() systemcmd.SystemSpec {
	return systemcmd.SystemSpec{
		Name:        "devops",
		Description: "BlueKing CI/CD (DevOps) build, CodeCC, and Stream commands",
		Subsystems: []systemcmd.SystemSpec{
			pipelinesystem.NewSystemSpec(),
			codeccsystem.NewSystemSpec(),
			streamsystem.NewSystemSpec(),
		},
	}
}
