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

package systemcmd

import syslib "github.com/TencentBlueKing/bk-cli/internal/system"

// ResolveRuntime builds the shared runtime for system commands from root-level deps.
func ResolveRuntime(deps BuildDeps) (*syslib.Runtime, error) {
	var (
		ctxOverride string
		dryRun      bool
		verbose     bool
		insecure    bool
	)
	if deps.GetContext != nil {
		ctxOverride = deps.GetContext()
	}
	if deps.IsDryRun != nil {
		dryRun = deps.IsDryRun()
	}
	if deps.IsVerbose != nil {
		verbose = deps.IsVerbose()
	}
	if deps.IsInsecure != nil {
		insecure = deps.IsInsecure()
	}
	return syslib.ResolveRuntimeWithOptions(ctxOverride, dryRun, verbose, syslib.RuntimeOptions{
		Insecure: insecure,
	})
}
