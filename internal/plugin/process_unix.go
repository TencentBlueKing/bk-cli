//go:build !windows

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
	"errors"
	"os"
	"syscall"
)

func hostSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP}
}

// forwardSignal drops terminal signals already delivered to the whole foreground group.
func forwardSignal(p *os.Process, sig os.Signal) {
	if sig == syscall.SIGINT || sig == syscall.SIGQUIT {
		return
	}
	_ = p.Signal(sig)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return !errors.Is(err, syscall.ESRCH) && !errors.Is(err, os.ErrProcessDone)
}

func exitCodeFromState(state *os.ProcessState) int {
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return state.ExitCode()
}
