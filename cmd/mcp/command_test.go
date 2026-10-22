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

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/output"
)

func TestMCP(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "MCP Command Suite") }

type unreadable struct{}

func (unreadable) Read([]byte) (int, error) { Fail("stdin must not be consumed"); return 0, io.EOF }

var _ = Describe("MCP command", func() {
	var calls, requests atomic.Int32
	var endpoint, dir string
	var result string
	var callDelay time.Duration
	run := func(args ...string) (string, string, error) {
		cmd := NewCommand()
		var out, stderr bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&stderr)
		cmd.SetIn(unreadable{})
		cmd.SetArgs(args)
		err := cmd.Execute()
		return out.String(), stderr.String(), err
	}
	BeforeEach(func() {
		calls.Store(0)
		requests.Store(0)
		dir = GinkgoT().TempDir()
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", dir)
		result = `{"content":[],"structuredContent":{"n":9007199254740993,"decimal":0.123456789012345678901}}`
		callDelay = 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			if r.Method == "DELETE" {
				w.WriteHeader(204)
				return
			}
			var req struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			w.Header().Set("Content-Type", "application/json")
			switch req.Method {
			case "server/discover":
				fmt.Fprintf(
					w,
					`{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"unsupported"}}`,
					req.ID,
				)
			case "initialize":
				fmt.Fprintf(
					w,
					`{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-11-25","capabilities":{"tools":{}},"serverInfo":{"name":"test","version":"1"}}}`,
					req.ID,
				)
			case "notifications/initialized", "notifications/cancelled":
				w.WriteHeader(202)
			case "tools/list":
				fmt.Fprintf(
					w,
					`{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"echo","description":"Echo","inputSchema":{"type":"object","required":["text"],"properties":{"text":{"type":"string"},"limit":{"type":"integer"},"enabled":{"type":"boolean"},"body":{"type":"string"}}}}]}}`,
					req.ID,
				)
			case "tools/call":
				calls.Add(1)
				if callDelay > 0 {
					select {
					case <-time.After(callDelay):
					case <-r.Context().Done():
						return
					}
				}
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":%s}`, req.ID, result)
			}
		}))
		DeferCleanup(server.Close)
		endpoint = server.URL
		writeConfig(dir, endpoint)
	})
	It("root and static server help do not connect or resolve credentials", func() {
		for _, args := range [][]string{{}, {"--help"}, {"test"}, {"test", "-h"}} {
			out, stderr, err := run(args...)
			Expect(err).NotTo(HaveOccurred())
			Expect(stderr).To(BeEmpty())
			Expect(out).To(ContainSubstring("Usage:"))
		}
		Expect(requests.Load()).To(BeZero())
		Expect(os.Remove(filepath.Join(dir, "mcp.json"))).To(Succeed())
		_, _, err := run("--help")
		Expect(err).NotTo(HaveOccurred())
	})
	It("tool help discovers but does not read a body or call", func() {
		out, _, err := run("test", "echo", "--help", "--body", "-")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("--text <string> required=true"))
		Expect(out).To(ContainSubstring("Reserved/complex"))
		Expect(calls.Load()).To(BeZero())
	})
	It("checks and lists tools using one final envelope", func() {
		for _, flag := range []string{"--check", "--tools"} {
			out, stderr, err := run("test", flag)
			Expect(err).NotTo(HaveOccurred())
			Expect(stderr).To(BeEmpty())
			Expect(out).To(ContainSubstring(`"complete": true`))
			Expect(out).To(ContainSubstring(`"tool_count": 1`))
		}
		Expect(calls.Load()).To(BeZero())
	})
	It("calls once and projects precise numbers", func() {
		out, stderr, err := run(
			"test",
			"echo",
			"--text",
			"00123",
			"--enabled=false",
			"--limit=0",
			"--jq",
			".data.result.structuredContent.decimal",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(stderr).To(BeEmpty())
		Expect(out).To(Equal("0.123456789012345678901\n"))
		Expect(calls.Load()).To(Equal(int32(1)))
	})
	It("does not retry a completed call when jq fails", func() {
		out, stderr, err := run("test", "echo", "--text", "value", "--jq", `error("display")`)
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring(`"structuredContent"`))
		Expect(stderr).To(ContainSubstring("jq_execution_error"))
		Expect(out).NotTo(ContainSubstring("execution_state"))
		Expect(calls.Load()).To(Equal(int32(1)))
	})
	It("reports post-discovery usage failures locally", func() {
		for _, args := range [][]string{{"test", "missing"}, {"test", "echo"}, {"test", "echo", "--text=x", "--limit=no"}, {"test", "echo", "--text=x", "--typo=x"}, {"test", "echo", "--text=a", "--text=b"}} {
			out, stderr, err := run(args...)
			Expect(err).To(HaveOccurred())
			Expect(out).To(BeEmpty())
			Expect(stderr).To(ContainSubstring(`"ok": false`))
			var cli *output.CLIError
			Expect(errors.As(err, &cli)).To(BeTrue())
			Expect(cli.ExitCode).To(Equal(1))
		}
		Expect(calls.Load()).To(BeZero())
	})
	It("rejects incompatible options before any request", func() {
		for _, args := range [][]string{{"test", "--context=x"}, {"test", "echo", "--check", "-h"}, {"test", "--tools", "--body={}"}, {"test", "echo", "--jq", "["}, {"test", "echo", "--timeout=0s"}, {"test", "--check", "--dry-run"}, {"test", "echo", "extra"}} {
			out, stderr, err := run(args...)
			Expect(err).To(HaveOccurred())
			Expect(out).To(BeEmpty())
			Expect(stderr).NotTo(BeEmpty())
		}
		Expect(requests.Load()).To(BeZero())
	})
	It("previews only a complete body offline and masks every header", func() {
		out, _, err := run("test", "not-discovered", "--body", `{"n":9007199254740993}`, "--dry-run")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`"dry_run": true`))
		Expect(out).To(ContainSubstring(`"X-Api-Key": "***"`))
		Expect(out).NotTo(ContainSubstring("secret-value"))
		Expect(out).To(ContainSubstring("9007199254740993"))
		for _, args := range [][]string{{"test", "echo", "--dry-run"}, {"test", "echo", "--dry-run", "--body={}", "--text=x"}} {
			_, _, err = run(args...)
			Expect(err).To(HaveOccurred())
		}
		Expect(requests.Load()).To(BeZero())
	})
	It("keeps tool errors and unsupported outcomes visible despite jq", func() {
		for _, raw := range []string{`{"content":[],"isError":true}`, `{"resultType":"input_required","inputRequests":{},"requestState":"pending"}`} {
			result = raw
			out, stderr, err := run("test", "echo", "--text=x", "--jq", "empty")
			Expect(err).To(HaveOccurred())
			Expect(out).To(ContainSubstring(`"ok": false`))
			Expect(out).To(ContainSubstring(`"result"`))
			Expect(out).NotTo(ContainSubstring("execution_state"))
			Expect(stderr).To(BeEmpty())
		}
	})
	It("marks a lost call response unknown without resending", func() {
		callDelay = time.Second
		out, stderr, err := run("test", "echo", "--text=x", "--timeout=100ms")
		Expect(err).To(HaveOccurred())
		Expect(stderr).To(BeEmpty())
		Expect(out).To(ContainSubstring(`"execution_state": "unknown"`))
		Expect(out).To(ContainSubstring("mcp_timeout"))
		Expect(calls.Load()).To(Equal(int32(1)))
	})
	It("honors caller cancellation", func() {
		cmd := NewCommand()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		cmd.SetContext(ctx)
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"test", "--check"})
		Expect(cmd.Execute()).To(HaveOccurred())
		Expect(calls.Load()).To(BeZero())
	})
	It("accepts stdin body once and ignores overridden named value types", func() {
		cmd := NewCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(io.Discard)
		cmd.SetIn(strings.NewReader(`{"text":"stdin"}`))
		cmd.SetArgs([]string{"test", "echo", "--limit=no", "--body", "-"})
		Expect(cmd.Execute()).To(Succeed())
		Expect(calls.Load()).To(Equal(int32(1)))
	})
})

func writeConfig(dir, endpoint string) {
	content := fmt.Sprintf(
		`{"mcpServers":{"test":{"type":"http","url":%q,"headers":{"X-Api-Key":"secret-value"}}}}`,
		endpoint,
	)
	Expect(os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(content), 0o600)).To(Succeed())
}
