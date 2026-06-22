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

// Package systemcmd provides shared types for system command registration.
package systemcmd

import (
	"io"

	"github.com/spf13/cobra"
)

// BuildDeps carries shared runtime inputs used when constructing system commands.
type BuildDeps struct {
	GetContext func() string
	IsDryRun   func() bool
	IsVerbose  func() bool
	IsInsecure func() bool
	WarnWriter io.Writer
}

// RegisterGoActionsFunc adds Go-implemented actions to a system command.
type RegisterGoActionsFunc func(parent *cobra.Command, deps BuildDeps) error

// SystemSpec defines a Go-owned system or subsystem command plus its optional action sources.
type SystemSpec struct {
	Name              string
	Description       string
	YAMLFile          string
	RegisterGoActions RegisterGoActionsFunc
	Subsystems        []SystemSpec
}
