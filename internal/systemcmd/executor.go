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
	"fmt"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
)

// ExecuteRequest runs a shared request and writes the final envelope to the command output.
func ExecuteRequest(
	cmd *cobra.Command,
	runtime *syslib.Runtime,
	actionName string,
	spec syslib.RequestSpec,
	mutate func(*output.Envelope) error,
) error {
	result, err := syslib.ExecuteRequest(runtime, spec)
	if err != nil {
		return err
	}
	if err := EnsureEnvelope(actionName, result.Envelope); err != nil {
		return err
	}
	if mutate != nil {
		if err := mutate(result.Envelope); err != nil {
			return err
		}
	}
	return result.Envelope.WriteJSON(cmd.OutOrStdout())
}

// EnsureEnvelope guards Go-implemented actions against unexpected nil envelopes.
func EnsureEnvelope(actionName string, env *output.Envelope) error {
	if env == nil {
		return fmt.Errorf("%s received an empty response envelope", actionName)
	}
	return nil
}
