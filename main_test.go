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

package main

import (
	"errors"
	"io"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	"github.com/TencentBlueKing/bk-cli/internal/plugin"
)

func captureStderr(fn func()) string {
	r, w, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())
	old := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stderr = old
	_ = w.Close()
	return <-done
}

var _ = Describe("exitCode", func() {
	It("returns 0 for success", func() {
		Expect(exitCode(nil)).To(Equal(0))
	})

	It("passes plugin exit status through without output", func() {
		var code int
		out := captureStderr(func() {
			code = exitCode(&plugin.ExitStatus{Code: 7})
		})
		Expect(code).To(Equal(7))
		Expect(out).To(BeEmpty())
	})

	It("keeps CLIError exit codes, including 125", func() {
		Expect(exitCode(&output.CLIError{
			ExitCode: 125,
			Code:     "plugin_not_installed",
		})).To(Equal(125))
	})

	It("reports other errors once as command_error", func() {
		var code int
		out := captureStderr(func() {
			code = exitCode(errors.New("boom"))
		})
		Expect(code).To(Equal(1))
		Expect(out).To(ContainSubstring(`"command_error"`))
	})
})
