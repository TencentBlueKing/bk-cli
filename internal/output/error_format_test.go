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

package output_test

import (
	"bytes"
	"io"
	"os"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/output"
)

func captureFileOutput(target **os.File, fn func()) string {
	orig := *target
	r, w, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())
	*target = w

	fn()

	Expect(w.Close()).To(Succeed())
	*target = orig

	data, readErr := io.ReadAll(r)
	Expect(readErr).NotTo(HaveOccurred())
	Expect(r.Close()).To(Succeed())
	return string(data)
}

var _ = Describe("error and format helpers", func() {
	It("formats CLI errors", func() {
		err := &output.CLIError{Code: "demo_error", Message: "broken"}
		Expect(err.Error()).To(Equal("demo_error: broken"))
	})

	It("writes envelopes to stdout", func() {
		env := output.Success("printed")

		written := captureFileOutput(&os.Stdout, func() {
			Expect(env.Print()).To(Succeed())
		})

		var parsed map[string]any
		Expect(json.Unmarshal([]byte(written), &parsed)).To(Succeed())
		Expect(parsed["message"]).To(Equal("printed"))
	})

	It("writes envelopes to stderr", func() {
		env := output.Err("demo_error", "broken", "fix it")

		written := captureFileOutput(&os.Stderr, func() {
			Expect(env.PrintErr()).To(Succeed())
		})

		var parsed map[string]any
		Expect(json.Unmarshal([]byte(written), &parsed)).To(Succeed())
		Expect(parsed["ok"]).To(BeFalse())
	})

	It("prints structured errors and returns CLI errors with the requested exit code", func() {
		var returned error
		written := captureFileOutput(&os.Stderr, func() {
			returned = output.PrintError(7, "boom", "bad things", "try again")
		})

		cliErr, ok := returned.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.ExitCode).To(Equal(7))
		Expect(cliErr.Code).To(Equal("boom"))
		Expect(cliErr.Message).To(Equal("bad things"))
		Expect(written).To(ContainSubstring(`"code": "boom"`))
	})

	It("creates user and system CLI errors", func() {
		userErr := output.UserError("invalid_argument", "bad input", "fix it")
		systemErr := output.SystemError("network_error", "downstream failed", "retry later")

		Expect(userErr).To(MatchError("invalid_argument: bad input"))
		Expect(systemErr).To(MatchError("network_error: downstream failed"))
	})

	It("resolves explicit and default output formats", func() {
		Expect(output.ResolveFormat("json")).To(Equal(output.FormatJSON))
		Expect(output.ResolveFormat("text")).To(Equal(output.FormatText))
		Expect(output.ResolveFormat("xml")).To(Equal(output.FormatJSON))
		Expect(output.ResolveFormat("")).To(Equal(output.FormatJSON))
	})

	It("reports non-interactive stdout when redirected", func() {
		interactive := false
		captureFileOutput(&os.Stdout, func() {
			interactive = output.IsInteractive()
		})

		Expect(interactive).To(BeFalse())
	})

	It("writes JSON through WriteJSON to arbitrary writers", func() {
		env := output.SuccessData(map[string]string{"result": "ok"})
		var buf bytes.Buffer

		Expect(env.WriteJSON(&buf)).To(Succeed())
		Expect(buf.String()).To(ContainSubstring(`"result": "ok"`))
	})
})
