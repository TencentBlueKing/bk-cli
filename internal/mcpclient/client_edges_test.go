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

package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MCP remote boundaries", func() {
	open := func(f *protocolFixture) *Client {
		server := httptest.NewServer(http.HandlerFunc(f.serve))
		DeferCleanup(server.Close)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		DeferCleanup(cancel)
		client := NewClient(ctx, server.URL, nil, false)
		DeferCleanup(client.Close)
		Expect(client.Connect()).To(Succeed())
		return client
	}
	DescribeTable(
		"preserves explicit failure state",
		func(raw, code string, known bool) {
			f := fixture()
			f.result = raw
			client := open(f)
			_, err := client.Discover()
			Expect(err).NotTo(HaveOccurred())
			result, err := client.Call("echo", map[string]any{})
			Expect(err).To(MatchError(ContainSubstring(code)))
			Expect(client.ResultKnown()).To(Equal(known))
			Expect(client.CallAttempted()).To(BeTrue())
			Expect(f.calls).To(Equal(1))
			if known {
				Expect(result).NotTo(BeNil())
			}
		},
		Entry("tool error", `{"content":[],"isError":true}`, "mcp_tool_error", true),
		Entry("task", `{"task":{"taskId":"pending"}}`, "mcp_capability_unsupported", true),
		Entry(
			"empty input",
			`{"resultType":"input_required","inputRequests":{}}`,
			"mcp_capability_unsupported",
			true,
		),
		Entry(
			"missing content",
			`{}`,
			"mcp_protocol_error",
			false,
		),
		Entry("wrong flag type", `{"content":[],"isError":"false"}`, "mcp_protocol_error", false),
		Entry(
			"non-object",
			`[]`,
			"mcp_protocol_error",
			false,
		),
		Entry("malformed", `{`, "mcp_protocol_error", false),
	)
	It("keeps complete JSON-RPC errors distinct from unknown tool outcomes", func() {
		f := fixture()
		f.rpcError = true
		client := open(f)
		_, err := client.Discover()
		Expect(err).NotTo(HaveOccurred())
		_, err = client.Call("echo", nil)
		Expect(err).To(MatchError(ContainSubstring("mcp_rpc_error")))
		Expect(client.ResultKnown()).To(BeTrue())
		Expect(err.Error()).NotTo(ContainSubstring("secret-error"))
		Expect(f.calls).To(Equal(1))
	})
	DescribeTable("closes the session after a failed call without replaying it", func(rpc bool) {
		f := fixture()
		if rpc {
			f.rpcError = true
		} else {
			f.callStatus = http.StatusServiceUnavailable
		}
		client := open(f)
		_, err := client.Call("echo", nil)
		Expect(err).To(HaveOccurred())
		client.Close()
		// Cleanup responses must not replace the failed operation's diagnostics.
		Expect(client.Headers()["Content-Type"]).To(Equal("application/json"))
		f.mu.Lock()
		defer f.mu.Unlock()
		Expect(f.methods).To(ContainElement("DELETE"))
		Expect(f.calls).To(Equal(1))
		Expect(f.headers[len(f.headers)-1].Get("Mcp-Session-Id")).To(Equal("fixture-session"))
	}, Entry("RPC error", true), Entry("HTTP error", false))
	It("accepts a null nextCursor as the final page", func() {
		f := fixture()
		f.list = strings.TrimSuffix(f.list, "}") + `,"nextCursor":null}`
		client := open(f)
		tools, err := client.Discover()
		Expect(err).NotTo(HaveOccurred())
		Expect(tools).To(HaveLen(1))
		Expect(f.listCount).To(Equal(1))
	})
	DescribeTable("keeps malformed JSON-RPC errors unknown", func(raw string) {
		f := fixture()
		f.rawError = raw
		client := open(f)
		_, err := client.Discover()
		Expect(err).NotTo(HaveOccurred())
		_, err = client.Call("echo", nil)
		Expect(err).To(MatchError(ContainSubstring("mcp_protocol_error")))
		Expect(client.ResultKnown()).To(BeFalse())
		Expect(f.calls).To(Equal(1))
	}, Entry("null", `null`), Entry("empty object", `{}`),
		Entry("missing code", `{"message":"bad"}`),
		Entry("fractional code", `{"code":1.5,"message":"bad"}`),
		Entry("missing message", `{"code":-32603}`))
	It("keeps the first SSE response selected by the SDK", func() {
		f := fixture()
		f.sse = true
		f.duplicateResult = true
		client := open(f)
		tools, err := client.Discover()
		Expect(err).NotTo(HaveOccurred())
		Expect(tools[0]["_meta"]).To(Equal(map[string]any{"n": json.Number("9007199254740993")}))
		result, err := client.Call("echo", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result["structuredContent"]).To(Equal(map[string]any{"n": json.Number("9007199254740993")}))
		Expect(f.calls).To(Equal(1))
	})
	DescribeTable("ignores non-message and empty SSE events exactly as the SDK does", func(prefix string) {
		f := fixture()
		f.sse = true
		f.eventPrefix = prefix
		client := open(f)
		_, err := client.Discover()
		Expect(err).NotTo(HaveOccurred())
		result, err := client.Call("echo", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result["structuredContent"]).To(Equal(map[string]any{"n": json.Number("9007199254740993")}))
		Expect(f.calls).To(Equal(1))
	}, Entry("named event", "event: ping\ndata: heartbeat\n\n"),
		Entry("empty data priming event", "id: initial\ndata:\n\n"))
	DescribeTable("never repeats a tool for HTTP errors", func(status int, code string) {
		f := fixture()
		f.callStatus = status
		client := open(f)
		_, err := client.Discover()
		Expect(err).NotTo(HaveOccurred())
		_, err = client.Call("echo", nil)
		Expect(err).To(MatchError(ContainSubstring(code)))
		Expect(f.calls).To(Equal(1))
	}, Entry("unauthorized", 401, "mcp_auth_required"), Entry("forbidden", 403, "mcp_permission_denied"), Entry("load shedding", 503, "mcp_protocol_error"))
	It("reports unsupported sampling without interactive handling or resending", func() {
		f := fixture()
		f.prefix = "data: {\"jsonrpc\":\"2.0\",\"id\":\"server-1\",\"method\":\"sampling/createMessage\",\"params\":{}}\n\n"
		client := open(f)
		_, err := client.Discover()
		Expect(err).NotTo(HaveOccurred())
		result, err := client.Call("echo", nil)
		Expect(err).To(MatchError(ContainSubstring("mcp_capability_unsupported")))
		Expect(result).To(HaveKey("unsupported_request"))
		Expect(f.calls).To(Equal(1))
	})
	It("does not resend after a disconnected SSE result stream", func() {
		f := fixture()
		f.prefix = ": keepalive\n\n"
		client := open(f)
		_, err := client.Discover()
		Expect(err).NotTo(HaveOccurred())
		_, err = client.Call("echo", nil)
		Expect(err).To(HaveOccurred())
		Expect(client.ResultKnown()).To(BeFalse())
		Expect(f.calls).To(Equal(1))
	})
	It("aggregates all pages including exact schema, content and result metadata", func() {
		f := fixture()
		f.listFn = func(page int) string {
			if page == 1 {
				return strings.TrimSuffix(f.list, "}") + `,"nextCursor":"next"}`
			}
			return `{"tools":[]}`
		}
		client := open(f)
		tools, err := client.Discover()
		Expect(err).NotTo(HaveOccurred())
		Expect(f.listCount).To(Equal(2))
		Expect(tools).To(HaveLen(1))
		encoded, err := json.Marshal(tools)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(encoded)).To(ContainSubstring(`"enum":[9007199254740993]`))
		result, err := client.Call("echo", nil)
		Expect(err).NotTo(HaveOccurred())
		encoded, err = json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(encoded)).To(ContainSubstring(`"_meta":{"n":9007199254740993}`))
		Expect(string(encoded)).To(ContainSubstring("0.1234567890123456789"))
		Expect(client.Connected()).To(BeTrue())
		Expect(client.ProtocolVersion()).To(Equal("2025-11-25"))
		Expect(client.Headers()).To(HaveKey("Content-Type"))
	})
	It("stops at the page limit and accepts exactly the limit when complete", func() {
		for _, complete := range []bool{false, true} {
			f := fixture()
			f.listFn = func(page int) string {
				if complete && page == MaxDiscoveryPages {
					return `{"tools":[]}`
				}
				return fmt.Sprintf(`{"tools":[],"nextCursor":"%d"}`, page)
			}
			client := open(f)
			tools, err := client.Discover()
			if complete {
				Expect(err).NotTo(HaveOccurred())
				Expect(tools).To(BeEmpty())
			} else {
				Expect(err).To(MatchError(ContainSubstring("mcp_discovery_incomplete")))
			}
			Expect(f.listCount).To(Equal(MaxDiscoveryPages))
			Expect(f.calls).To(BeZero())
		}
	})
	It("bounds the accumulated tool catalog before any tool call", func() {
		f := fixture()
		description := strings.Repeat("x", 12<<20)
		f.listFn = func(page int) string {
			return fmt.Sprintf(
				`{"tools":[{"name":"t%d","description":%q,"inputSchema":{}}],"nextCursor":"%d"}`,
				page,
				description,
				page,
			)
		}
		client := open(f)
		_, err := client.Discover()
		Expect(err).To(MatchError(ContainSubstring("mcp_response_too_large")))
		Expect(f.listCount).To(Equal(3))
		Expect(f.calls).To(BeZero())
	})
	DescribeTable("rejects invalid tool directories", func(list string) {
		f := fixture()
		f.list = list
		client := open(f)
		_, err := client.Discover()
		Expect(err).To(HaveOccurred())
		Expect(f.calls).To(BeZero())
	}, Entry("missing list", `{}`), Entry("bad tool", `{"tools":[null]}`), Entry("no schema", `{"tools":[{"name":"x"}]}`), Entry("bad cursor", `{"tools":[],"nextCursor":4}`))
	It("uses a total deadline and releases a timed out request", func() {
		f := fixture()
		f.delay = time.Second
		server := httptest.NewServer(http.HandlerFunc(f.serve))
		DeferCleanup(server.Close)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		DeferCleanup(cancel)
		client := NewClient(ctx, server.URL, nil, false)
		DeferCleanup(client.Close)
		Expect(client.Connect()).To(Succeed())
		_, err := client.Discover()
		Expect(err).NotTo(HaveOccurred())
		_, err = client.Call("echo", nil)
		Expect(err).To(MatchError(ContainSubstring("mcp_timeout")))
		Expect(client.ResultKnown()).To(BeFalse())
		f.mu.Lock()
		defer f.mu.Unlock()
		Expect(f.calls).To(Equal(1))
	})
	It("blocks redirects without forwarding any credentials", func() {
		reached := false
		target := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }),
		)
		DeferCleanup(target.Close)
		server := httptest.NewServer(
			http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) },
			),
		)
		DeferCleanup(server.Close)
		client := NewClient(context.Background(), server.URL, http.Header{"X-Key": []string{"secret"}}, false)
		DeferCleanup(client.Close)
		Expect(client.Connect()).To(MatchError(ContainSubstring("mcp_connection_error")))
		Expect(reached).To(BeFalse())
	})
	It("requires explicit insecure for an untrusted TLS server", func() {
		f := fixture()
		server := httptest.NewTLSServer(http.HandlerFunc(f.serve))
		DeferCleanup(server.Close)
		for _, insecure := range []bool{false, true} {
			client := NewClient(context.Background(), server.URL, nil, insecure)
			DeferCleanup(client.Close)
			err := client.Connect()
			if insecure {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(MatchError(ContainSubstring("mcp_connection_error")))
			}
		}
	})
	It("masks configured and known response credentials case-insensitively", func() {
		masked := maskedResponseHeaders(
			http.Header{
				"x-api-key":             []string{"secret"},
				"Authorization":         []string{"Bearer secret"},
				"X-Bkapi-Authorization": []string{"secret"},
				"Set-Cookie":            []string{"secret"},
				"mCp-SeSsIoN-iD":        []string{"live-session"},
				"Content-Type":          []string{"application/json"},
			},
			http.Header{"X-Api-Key": []string{""}},
		)
		Expect(masked["x-api-key"]).To(Equal("***"))
		Expect(masked["Authorization"]).To(Equal("***"))
		Expect(masked["X-Bkapi-Authorization"]).To(Equal("***"))
		Expect(masked["Set-Cookie"]).To(Equal("***"))
		Expect(masked["mCp-SeSsIoN-iD"]).To(Equal("***"))
		Expect(masked["Content-Type"]).To(Equal("application/json"))
	})
	It("enforces exact JSON and SSE read limits without success truncation", func() {
		for _, sse := range []bool{false, true} {
			for _, extra := range []int{0, 1} {
				state := &wireState{}
				prefix := `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`
				if sse {
					prefix = "data: " + prefix
					prefix += strings.Repeat(" ", MaxResponseBytes-len(prefix)-2) + "\n\n"
				} else {
					prefix += strings.Repeat(" ", MaxResponseBytes-len(prefix))
				}
				if extra > 0 {
					prefix = " " + prefix
				}
				body := &observedBody{
					source: io.NopCloser(bytes.NewBufferString(prefix)),
					state:  state,
					method: "tools/list",
					id:     json.RawMessage("1"),
					sse:    sse,
				}
				_, err := io.Copy(io.Discard, body)
				Expect(body.Close()).To(Succeed())
				if extra == 0 {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(MatchError(ContainSubstring("mcp_response_too_large")))
				}
			}
		}
	})
})
