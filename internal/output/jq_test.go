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

package output

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

type jqShortWriter struct{}

func (*jqShortWriter) Write(p []byte) (int, error) { return len(p) / 2, nil }

type jqFailWriter struct{ calls int }

func (w *jqFailWriter) Write(p []byte) (int, error) { w.calls++; return len(p) / 2, io.ErrClosedPipe }

var _ = Describe("jq final envelope output", func() {
	var cmd *cobra.Command
	var stdout, stderr bytes.Buffer
	var env *Envelope
	BeforeEach(func() {
		stdout.Reset()
		stderr.Reset()
		cmd = &cobra.Command{Use: "action", SilenceErrors: true, SilenceUsage: true}
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		AddJQFlag(cmd)
		env = APIResponse(200, map[string]string{"X-Request-Id": "trace"}, map[string]any{
			"id": json.Number("18446744073709551617"), "name": "蓝鲸\nCLI", "items": []any{1, 2},
		})
	})
	It("retains the exact default envelope and bypasses projection for known failures", func() {
		var expected bytes.Buffer
		Expect(env.WriteJSON(&expected)).To(Succeed())
		Expect(WriteCommandEnvelope(cmd, env)).To(Succeed())
		Expect(stdout.String()).To(Equal(expected.String()))
		stdout.Reset()
		expected.Reset()
		env.OK = false
		Expect(cmd.Flags().Set("jq", "empty")).To(Succeed())
		Expect(env.WriteJSON(&expected)).To(Succeed())
		Expect(WriteCommandEnvelope(cmd, env)).To(Succeed())
		Expect(stdout.String()).To(Equal(expected.String()))
	})
	DescribeTable(
		"encodes results as JSON lines from the full envelope",
		func(expr, expected string) {
			Expect(cmd.Flags().Set("jq", expr)).To(Succeed())
			Expect(WriteCommandEnvelope(cmd, env)).To(Succeed())
			Expect(stdout.String()).To(Equal(expected))
			Expect(stderr.Len()).To(BeZero())
		},
		Entry("integer", ".data.id", "18446744073709551617\n"),
		Entry("arithmetic integer", ".data.id + 1", "18446744073709551618\n"),
		Entry("string", ".data.name", "\"蓝鲸\\nCLI\"\n"),
		Entry("array", ".data.items", "[1,2]\n"),
		Entry(
			"object",
			"{ok,status,trace:.headers[\"X-Request-Id\"]}",
			"{\"ok\":true,\"status\":200,\"trace\":\"trace\"}\n",
		),
		Entry("multiple", ".data.items[]", "1\n2\n"),
		Entry("null", "null", "null\n"),
		Entry("false", "false", "false\n"),
		Entry("empty", "empty", ""),
		Entry(
			"halt",
			"1, halt, 2",
			"1\n",
		),
		Entry("environment", "env", "{}\n"),
		Entry("ENV variable", "$ENV", "{}\n"),
		Entry("nested halt", "[1, halt, 2]", ""),
		Entry("try halt", "try halt catch true", ""),
	)
	DescribeTable("rejects invalid input before command hooks", func(expr string) {
		ran := false
		cmd.PreRun = func(*cobra.Command, []string) { ran = true }
		cmd.RunE = func(*cobra.Command, []string) error { ran = true; return nil }
		cmd.SetArgs([]string{"--jq", expr})
		Expect(cmd.Execute()).NotTo(Succeed())
		Expect(ran).To(BeFalse())
	}, Entry("empty", ""), Entry("whitespace", "  "), Entry("parse", ".["), Entry("compile", "missing_function"),
		Entry("stdin", "input"), Entry("modules", "include \"secret\"; ."))
	DescribeTable("falls back atomically and reports projection errors", func(expr, code string) {
		Expect(cmd.Flags().Set("jq", expr)).To(Succeed())
		err := WriteCommandEnvelope(cmd, env)
		var cliErr *CLIError
		Expect(errors.As(err, &cliErr)).To(BeTrue())
		Expect(cliErr.ExitCode).To(Equal(2))
		Expect(cliErr.Code).To(Equal(code))
		var expected bytes.Buffer
		Expect(env.WriteJSON(&expected)).To(Succeed())
		Expect(stdout.String()).To(Equal(expected.String()))
		Expect(stderr.String()).To(ContainSubstring(code))
		Expect(stderr.String()).To(ContainSubstring("request has already completed"))
	}, Entry("type", "1, .data.id[]", "jq_execution_error"),
		Entry("error", "error(\"bad\")", "jq_execution_error"),
		Entry("halt error", "halt_error", "jq_execution_error"),
		Entry("null zero halt error", "null | halt_error(0)", "jq_execution_error"),
		Entry("uncatchable halt error", "try (null | halt_error(0)) catch true", "jq_execution_error"),
		Entry("results", "range(0; 10001)", "jq_limit_exceeded"),
		Entry("bytes", "\"x\" * 16777216", "jq_limit_exceeded"))
	It("bounds infinite execution and honors caller cancellation", func() {
		Expect(cmd.Flags().Set("jq", "def spin: spin; spin")).To(Succeed())
		started := time.Now()
		err := WriteCommandEnvelope(cmd, env)
		Expect(err).To(MatchError(ContainSubstring("jq_limit_exceeded")))
		Expect(time.Since(started)).To(BeNumerically("<", 5*time.Second))
		stdout.Reset()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		cmd.SetContext(ctx)
		Expect(cmd.Flags().Set("jq", ".")).To(Succeed())
		Expect(WriteCommandEnvelope(cmd, env)).To(MatchError(ContainSubstring("jq_execution_error")))
	})
	It("propagates writer errors without appending fallback data", func() {
		Expect(cmd.Flags().Set("jq", ".data.items[]")).To(Succeed())
		w := &jqFailWriter{}
		cmd.SetOut(w)
		Expect(WriteCommandEnvelope(cmd, env)).To(MatchError(io.ErrClosedPipe))
		Expect(w.calls).To(Equal(1))
	})
	It("accepts exactly the result limit and propagates short and diagnostic writes", func() {
		Expect(cmd.Flags().Set("jq", "range(0; 10000)")).To(Succeed())
		Expect(WriteCommandEnvelope(cmd, env)).To(Succeed())
		Expect(strings.Count(stdout.String(), "\n")).To(Equal(jqMaxResults))
		cmd.SetOut(&jqShortWriter{})
		Expect(WriteCommandEnvelope(cmd, env)).To(MatchError(io.ErrShortWrite))
		cmd.SetOut(&stdout)
		cmd.SetErr(&jqFailWriter{})
		Expect(cmd.Flags().Set("jq", "error(\"bad\")")).To(Succeed())
		Expect(WriteCommandEnvelope(cmd, env)).To(MatchError(io.ErrClosedPipe))
	})
	It("does not touch stdout when the projection is empty", func() {
		Expect(cmd.Flags().Set("jq", "empty")).To(Succeed())
		writer := &jqFailWriter{}
		cmd.SetOut(writer)
		Expect(WriteCommandEnvelope(cmd, env)).To(Succeed())
		Expect(writer.calls).To(BeZero())
	})

	It("does not claim a dry-run request has executed on projection failure", func() {
		env = DryRun(&DryRunRequest{Method: "POST", URL: "https://example.com"})
		Expect(cmd.Flags().Set("jq", "error(\"bad\")")).To(Succeed())
		Expect(WriteCommandEnvelope(cmd, env)).NotTo(Succeed())
		Expect(stderr.String()).NotTo(ContainSubstring("request has already completed"))
	})
	It("does not let input conversion errors discard a completed result", func() {
		env.Data = map[string]any{"id": json.Number("1e400")}
		Expect(cmd.Flags().Set("jq", ".data.id[]")).To(Succeed())
		Expect(WriteCommandEnvelope(cmd, env)).NotTo(Succeed())
		Expect(stdout.String()).To(ContainSubstring("1e400"))
		Expect(strings.HasPrefix(stdout.String(), "{\n")).To(BeTrue())
	})
})
