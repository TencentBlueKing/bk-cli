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
	var rpcError bool
	var callDelay time.Duration
	var receivedHeaders chan http.Header
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
		rpcError = false
		receivedHeaders = make(chan http.Header, 64)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			receivedHeaders <- r.Header.Clone()
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
				if rpcError {
					w.Header().Set("Mcp-Session-Id", "private-session")
					fmt.Fprintf(
						w,
						`{"jsonrpc":"2.0","id":%s,"error":{"code":-32602,"message":"Unknown tool"}}`,
						req.ID,
					)
					return
				}
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
	It("preserves RPC code and message in a known failure envelope", func() {
		rpcError = true
		out, stderr, err := run("test", "echo", "--text", "hello")
		Expect(err).To(HaveOccurred())
		Expect(stderr).To(BeEmpty())
		var env struct {
			Error struct{ Code string }
			Data  struct {
				RPCError struct {
					Code    int
					Message string
				} `json:"rpc_error"`
				ExecutionState string `json:"execution_state"`
			}
			Headers map[string]string
		}
		Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
		Expect(env.Error.Code).To(Equal("mcp_rpc_error"))
		Expect(env.Data.RPCError.Code).To(Equal(-32602))
		Expect(env.Data.RPCError.Message).To(Equal("Unknown tool"))
		Expect(env.Data.ExecutionState).To(BeEmpty())
		Expect(out).NotTo(ContainSubstring("private-session"))
		Expect(env.Headers["Mcp-Session-Id"]).To(Equal("***"))
		Expect(calls.Load()).To(Equal(int32(1)))
	})
	It("rejects dry-run combined with help before network or stdin access", func() {
		out, stderr, err := run("test", "echo", "--dry-run", "--help", "--body", "-")
		Expect(err).To(HaveOccurred())
		Expect(out).To(BeEmpty())
		Expect(stderr).To(ContainSubstring("invalid_mcp_arguments"))
		Expect(requests.Load()).To(BeZero())
	})
	It("distinguishes local authentication errors from structural listing", func() {
		path := filepath.Join(dir, "mcp.json")
		content := fmt.Sprintf(
			`{"mcpServers":{"test":{"type":"http","url":%q,"headers":{"X-Bkapi-Authorization":"{\"bk_app_code\":\"app\",\"bk_app_secret\":\"SECRET\"}"}}}}`,
			endpoint,
		)
		Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
		_, _, err := run("--list")
		Expect(err).NotTo(HaveOccurred())
		out, stderr, err := run("test", "--check")
		Expect(err).To(HaveOccurred())
		Expect(out).To(BeEmpty())
		Expect(
			stderr,
		).To(
			And(
				ContainSubstring("invalid_mcp_auth"),
				ContainSubstring("test"),
				ContainSubstring("bk_ticket"),
				ContainSubstring("bk_token"),
				ContainSubstring("access_token"),
			),
		)
		Expect(stderr).NotTo(ContainSubstring("SECRET"))
		Expect(requests.Load()).To(BeZero())
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
	It("lists configured servers offline with stable names and masked headers", func() {
		path := filepath.Join(dir, "other.json")
		content := `{"mcpServers":{"zeta":{"type":"http","url":"http://127.0.0.1:1/z","headers":{"Authorization":"Bearer ${UNSET}"}},"alpha":{"type":"http","url":"http://127.0.0.1:1/a","headers":{"X-Key":"private-secret"}}}}`
		Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
		out, stderr, err := run("--list", "--config", path)
		Expect(err).NotTo(HaveOccurred())
		Expect(stderr).To(BeEmpty())
		var env struct {
			Data struct {
				ConfigFile  string `json:"config_file"`
				ServerCount int    `json:"server_count"`
				Servers     []struct {
					Name, URL, Transport string
					Headers              map[string]string
				} `json:"servers"`
			}
		}
		Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
		Expect(env.Data.ConfigFile).To(Equal(path))
		Expect(env.Data.ServerCount).To(Equal(2))
		Expect(env.Data.Servers).To(HaveLen(2))
		Expect(env.Data.Servers[0].Name).To(Equal("alpha"))
		Expect(env.Data.Servers[0].URL).To(Equal("http://127.0.0.1:1/a"))
		Expect(env.Data.Servers[0].Transport).To(Equal("http"))
		Expect(env.Data.Servers[0].Headers).To(Equal(map[string]string{"X-Key": "***"}))
		Expect(env.Data.Servers[1].Name).To(Equal("zeta"))
		Expect(out).NotTo(ContainSubstring("private-secret"))
		Expect(out).NotTo(ContainSubstring("${UNSET}"))
		out, _, err = run("--list", "--jq", ".data.servers[].name")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("\"test\"\n"))
		Expect(requests.Load()).To(BeZero())
	})
	It("lists empty configuration and reports missing or malformed files locally", func() {
		path := filepath.Join(dir, "mcp.json")
		Expect(os.WriteFile(path, []byte(`{"mcpServers":{}}`), 0o600)).To(Succeed())
		out, _, err := run("--list", "--jq", ".data.servers")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("[]\n"))
		Expect(os.WriteFile(path, []byte(`{"mcpServers":{"bad":{}}}`), 0o600)).To(Succeed())
		out, stderr, err := run("--list")
		Expect(err).To(HaveOccurred())
		Expect(out).To(BeEmpty())
		Expect(stderr).To(ContainSubstring("invalid_mcp_config"))
		Expect(os.Remove(path)).To(Succeed())
		out, stderr, err = run("--list")
		Expect(err).To(HaveOccurred())
		Expect(out).To(BeEmpty())
		Expect(stderr).To(ContainSubstring("mcp_config_not_found"))
		out, _, err = run("--list", "--help")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("Usage:"))
		Expect(requests.Load()).To(BeZero())
	})
	It("rejects list mode combined with a target or remote execution", func() {
		for _, args := range [][]string{{"--list", "test"}, {"--list", "--check"}, {"--list", "--tools"}, {"--list", "--body={}"}, {"--list", "--dry-run"}, {"--list", "--text=x"}, {"--list", "--context=x"}} {
			out, _, err := run(args...)
			Expect(err).To(HaveOccurred())
			Expect(out).To(BeEmpty())
		}
		Expect(requests.Load()).To(BeZero())
	})
	It("sends configured headers literally without environment rendering", func() {
		GinkgoT().Setenv("MCP_LITERAL_SECRET", "changed-by-environment")
		content := fmt.Sprintf(
			`{"mcpServers":{"test":{"type":"http","url":%q,"headers":{"X-Key":"${MCP_LITERAL_SECRET}/$${missing}"}}}}`,
			endpoint,
		)
		Expect(os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(content), 0o600)).To(Succeed())
		_, _, err := run("test", "echo", "--text=x")
		Expect(err).NotTo(HaveOccurred())
		Expect(len(receivedHeaders)).To(BeNumerically(">", 0))
		for len(receivedHeaders) > 0 {
			Expect((<-receivedHeaders).Get("X-Key")).To(Equal("${MCP_LITERAL_SECRET}/$${missing}"))
		}
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
