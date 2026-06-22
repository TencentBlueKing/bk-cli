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

package system

import "github.com/TencentBlueKing/bk-cli/internal/requestexec"

// Runtime contains resolved context state shared across one or more API calls.
type Runtime = requestexec.Runtime

// RequestSpec describes a single outbound API request made by YAML or Go-implemented system commands.
type RequestSpec = requestexec.RequestSpec

// RequestResult contains the envelope produced by a single API request.
type RequestResult = requestexec.RequestResult

// RuntimeOptions controls request execution behavior beyond context/output flags.
type RuntimeOptions = requestexec.RuntimeOptions

// ResolveRuntime loads the active context config for later request execution.
func ResolveRuntime(ctxOverride string, dryRun, verbose bool) (*Runtime, error) {
	return requestexec.ResolveRuntime(ctxOverride, dryRun, verbose)
}

// ResolveRuntimeWithOptions loads the active context config with request-level options.
func ResolveRuntimeWithOptions(
	ctxOverride string,
	dryRun, verbose bool,
	opts RuntimeOptions,
) (*Runtime, error) {
	return requestexec.ResolveRuntimeWithOptions(ctxOverride, dryRun, verbose, opts)
}

// ExecuteRequest builds, validates, and optionally executes a single request without printing.
func ExecuteRequest(runtime *Runtime, spec RequestSpec) (*RequestResult, error) {
	return requestexec.ExecuteRequest(runtime, spec)
}
