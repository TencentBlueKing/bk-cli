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
	"syscall"
)

// Win32 values and rules kept platform-neutral so they are tested on the Linux CI; only
// process_windows.go uses them in non-test builds.
//
//nolint:unused // used by process_windows.go
const (
	windowsErrorAccessDenied     syscall.Errno = 5
	windowsErrorInvalidParameter syscall.Errno = 87
	windowsStillActive           uint32        = 259
)

// windowsOpenErrorAlive fails closed: only ERROR_INVALID_PARAMETER proves the PID does not exist.
// Access denied (for example an elevated holder seen from a non-elevated process) keeps the lock.
//
//nolint:unused // used by process_windows.go
func windowsOpenErrorAlive(err error) bool {
	return !errors.Is(err, windowsErrorInvalidParameter)
}

// windowsExitCodeAlive treats an openable process as alive until it reports a real exit code.
//
//nolint:unused // used by process_windows.go
func windowsExitCodeAlive(code uint32, err error) bool {
	return err != nil || code == windowsStillActive
}
