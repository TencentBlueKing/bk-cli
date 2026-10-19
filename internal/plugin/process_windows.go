//go:build windows

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

package plugin

import (
	"math"
	"os"
	"syscall"
)

// processQueryLimitedInformation is PROCESS_QUERY_LIMITED_INFORMATION, which is granted across
// integrity levels unlike the PROCESS_QUERY_INFORMATION right used by os.FindProcess.
const processQueryLimitedInformation = 0x1000

func hostSignals() []os.Signal { return []os.Signal{os.Interrupt} }

// forwardSignal drops Ctrl-C/Ctrl-Break because the console delivers it to the child directly.
func forwardSignal(_ *os.Process, _ os.Signal) {}

func processAlive(pid int) bool {
	if pid <= 0 || int64(pid) > math.MaxUint32 {
		return false
	}
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return windowsOpenErrorAlive(err)
	}
	defer func() { _ = syscall.CloseHandle(h) }()
	var code uint32
	err = syscall.GetExitCodeProcess(h, &code)
	return windowsExitCodeAlive(code, err)
}

func exitCodeFromState(state *os.ProcessState) int { return state.ExitCode() }
