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

import (
	"github.com/spf13/cobra"

	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
)

// AddCommonRequestFlags registers the shared request flags used by Go-implemented system actions.
func AddCommonRequestFlags(cmd *cobra.Command, stage, body *string, headers *[]string) {
	cmd.Flags().StringVar(
		stage,
		syslib.ActionStageFlagName,
		"prod",
		"[common] Optional; Override API gateway stage when provided",
	)
	cmd.Flags().StringVar(
		body,
		syslib.ActionBodyFlagName,
		"",
		"[common] Optional; JSON request body; Overrides synthesized body inputs when provided",
	)
	cmd.Flags().StringArrayVar(
		headers,
		syslib.ActionHeaderFlagName,
		nil,
		"[common] Optional; Additional headers (key:value, repeatable; auth/tenant overrides allowed)",
	)
}

// AddCommonRequestFlagsWithoutBody registers the shared request flags for actions without --body.
func AddCommonRequestFlagsWithoutBody(cmd *cobra.Command, stage *string, headers *[]string) {
	cmd.Flags().StringVar(
		stage,
		syslib.ActionStageFlagName,
		"prod",
		"[common] Optional; Override API gateway stage when provided",
	)
	cmd.Flags().StringArrayVar(
		headers,
		syslib.ActionHeaderFlagName,
		nil,
		"[common] Optional; Additional headers (key:value, repeatable; auth/tenant overrides allowed)",
	)
}
