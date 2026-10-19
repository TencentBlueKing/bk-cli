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
	"fmt"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Windows lock holder liveness", func() {
	It("treats only a nonexistent PID as dead when the process cannot be opened", func() {
		Expect(windowsOpenErrorAlive(windowsErrorInvalidParameter)).To(BeFalse())
		Expect(windowsOpenErrorAlive(fmt.Errorf("OpenProcess: %w", windowsErrorInvalidParameter))).To(BeFalse())
		Expect(windowsOpenErrorAlive(windowsErrorAccessDenied)).To(BeTrue())
		Expect(windowsOpenErrorAlive(errors.New("unexpected"))).To(BeTrue())
	})

	It("treats an opened process as alive only while its exit code is STILL_ACTIVE", func() {
		Expect(windowsExitCodeAlive(windowsStillActive, nil)).To(BeTrue())
		Expect(windowsExitCodeAlive(0, nil)).To(BeFalse())
		Expect(windowsExitCodeAlive(1, nil)).To(BeFalse())
		Expect(windowsExitCodeAlive(0, syscall.Errno(1))).To(BeTrue())
	})
})
