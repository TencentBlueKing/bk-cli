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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type protocolFixture struct {
	mu              sync.Mutex
	methods         []string
	calls           int
	version         string
	sse             bool
	list            string
	result          string
	status          int
	delay           time.Duration
	headers         []http.Header
	arguments       json.RawMessage
	listFn          func(int) string
	listCount       int
	callStatus      int
	prefix          string
	rpcError        bool
	rawError        string
	eventPrefix     string
	duplicateResult bool
	sseEnding       *string
	responseStatus  int
}

func (f *protocolFixture) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		f.mu.Lock()
		f.methods = append(f.methods, "DELETE")
		f.headers = append(f.headers, r.Header.Clone())
		f.mu.Unlock()
		w.WriteHeader(204)
		return
	}
	var request struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params struct {
			Arguments json.RawMessage `json:"arguments"`
		} `json:"params"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil {
		w.WriteHeader(400)
		return
	}
	f.mu.Lock()
	f.methods = append(f.methods, request.Method)
	f.headers = append(f.headers, r.Header.Clone())
	if request.Method == "tools/call" {
		f.calls++
		f.arguments = request.Params.Arguments
	}
	f.mu.Unlock()
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	var result string
	switch request.Method {
	case "server/discover":
		if f.version != "2026-07-28" {
			fmt.Fprintf(
				w,
				`{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"unsupported"}}`,
				request.ID,
			)
			return
		}
		result = `{"supportedVersions":["2026-07-28"],"capabilities":{"tools":{}},"_meta":{"io.modelcontextprotocol/serverInfo":{"name":"fixture","version":"1"}}}`
	case "initialize":
		w.Header().Set("Mcp-Session-Id", "fixture-session")
		result = fmt.Sprintf(
			`{"protocolVersion":%q,"capabilities":{"tools":{}},"serverInfo":{"name":"fixture","version":"1"}}`,
			f.version,
		)
	case "notifications/initialized", "notifications/cancelled":
		w.WriteHeader(202)
		return
	case "tools/list":
		f.listCount++
		result = f.list
		if f.listFn != nil {
			result = f.listFn(f.listCount)
		}
	case "tools/call":
		if f.callStatus > 0 {
			w.WriteHeader(f.callStatus)
			return
		}
		if f.delay > 0 {
			select {
			case <-time.After(f.delay):
			case <-r.Context().Done():
				return
			}
		}
		result = f.result
	default:
		w.WriteHeader(400)
		return
	}
	message := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":%s}`, request.ID, result)
	if f.rpcError && request.Method == "tools/call" {
		message = fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%s,"error":{"code":-32602,"message":"secret-error"}}`,
			request.ID,
		)
	}
	if f.rawError != "" && request.Method == "tools/call" {
		message = fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"error":%s}`, request.ID, f.rawError)
	}
	if f.prefix != "" && request.Method == "tools/call" {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, f.prefix)
		return
	}
	if f.responseStatus != 0 && request.Method == "tools/call" {
		w.WriteHeader(f.responseStatus)
	}
	if f.sse {
		w.Header().Set("Content-Type", "text/event-stream")
		ending := "\r\n\r\n"
		if f.sseEnding != nil {
			ending = *f.sseEnding
		}
		fmt.Fprintf(w, "%sevent: message\r\ndata: %s%s", f.eventPrefix, message, ending)
		if f.duplicateResult {
			fmt.Fprintf(
				w,
				"event: message\ndata: %s\n\n",
				strings.ReplaceAll(message, "9007199254740993", "42"),
			)
		}
	} else {
		fmt.Fprint(w, message)
	}
}

func fixture() *protocolFixture {
	return &protocolFixture{
		version: "2025-11-25",
		list:    `{"tools":[{"name":"echo","inputSchema":{"type":"object","properties":{"n":{"type":"integer","enum":[9007199254740993]}}},"_meta":{"n":9007199254740993}}]}`,
		result:  `{"content":[{"type":"text","text":"ok","_meta":{"n":9007199254740993}}],"structuredContent":{"n":9007199254740993},"_meta":{"decimal":0.1234567890123456789}}`,
	}
}

var _ = Describe("MCP protocol client", func() {
	DescribeTable("negotiates and preserves raw values with exactly one tool call", func(version string, sse bool) {
		f := fixture()
		f.version = version
		f.sse = sse
		server := httptest.NewServer(http.HandlerFunc(f.serve))
		DeferCleanup(server.Close)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		DeferCleanup(cancel)
		client := NewClient(ctx, server.URL, http.Header{"Authorization": []string{"Bearer fixture"}}, false)
		DeferCleanup(client.Close)
		Expect(client.Connect()).To(Succeed())
		tools, err := client.Discover()
		Expect(err).NotTo(HaveOccurred())
		Expect(tools).To(HaveLen(1))
		Expect(tools[0]["_meta"]).To(Equal(map[string]any{"n": json.Number("9007199254740993")}))
		result, err := client.Call("echo", map[string]any{"n": json.Number("9007199254740993")})
		Expect(err).NotTo(HaveOccurred())
		Expect(result["_meta"]).To(Equal(map[string]any{"decimal": json.Number("0.1234567890123456789")}))
		f.mu.Lock()
		defer f.mu.Unlock()
		Expect(f.calls).To(Equal(1))
		Expect(string(f.arguments)).To(ContainSubstring("9007199254740993"))
		for _, headers := range f.headers {
			Expect(headers.Get("Authorization")).To(Equal("Bearer fixture"))
		}
	}, Entry("2025 March JSON", "2025-03-26", false), Entry("2025 June SSE", "2025-06-18", true), Entry("2025 November JSON", "2025-11-25", false), Entry("2026 July SSE", "2026-07-28", true))
	DescribeTable(
		"uses only the selected identity for initialization, every page, call and cleanup",
		func(name, value string) {
			f := fixture()
			f.listFn = func(page int) string {
				if page == 1 {
					return `{"tools":[],"nextCursor":"second"}`
				}
				return f.list
			}
			server := httptest.NewServer(http.HandlerFunc(f.serve))
			DeferCleanup(server.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			DeferCleanup(cancel)
			headers := make(http.Header)
			if name != "" {
				headers.Set(name, value)
			}
			client := NewClient(ctx, server.URL, headers, false)
			DeferCleanup(client.Close)
			Expect(client.Connect()).To(Succeed())
			_, err := client.Discover()
			Expect(err).NotTo(HaveOccurred())
			_, err = client.Call("echo", nil)
			Expect(err).NotTo(HaveOccurred())
			client.Close()
			f.mu.Lock()
			defer f.mu.Unlock()
			Expect(f.methods).To(ContainElements("initialize", "tools/list", "tools/call", "DELETE"))
			Expect(f.listCount).To(Equal(2))
			Expect(f.calls).To(Equal(1))
			for _, actual := range f.headers {
				Expect(actual.Get("Authorization")).To(Equal(headers.Get("Authorization")))
				Expect(
					actual.Get("X-Bkapi-Authorization"),
				).To(
					Equal(headers.Get("X-Bkapi-Authorization")),
				)
				Expect(actual.Get("X-Bk-Tenant-Id")).To(BeEmpty())
			}
		},
		Entry("personal", "Authorization", "Bearer personal"),
		Entry(
			"application ticket",
			"X-Bkapi-Authorization",
			`{"bk_app_code":"app","bk_app_secret":"secret","bk_ticket":"ticket"}`,
		),
		Entry(
			"application token",
			"X-Bkapi-Authorization",
			`{"bk_app_code":"app","bk_app_secret":"secret","bk_token":"token"}`,
		),
		Entry("access token", "X-Bkapi-Authorization", `{"access_token":"access"}`),
		Entry("anonymous", "", ""),
	)
	It("does not retry authentication failures", func() {
		f := fixture()
		f.status = 401
		server := httptest.NewServer(http.HandlerFunc(f.serve))
		DeferCleanup(server.Close)
		client := NewClient(context.Background(), server.URL, nil, false)
		DeferCleanup(client.Close)
		Expect(client.Connect()).To(MatchError(ContainSubstring("mcp_auth_required")))
		Expect(f.methods).To(HaveLen(1))
	})
	It("rejects duplicate tools and never calls a tool", func() {
		f := fixture()
		f.list = `{"tools":[{"name":"x","inputSchema":{}},{"name":"x","inputSchema":{}}]}`
		server := httptest.NewServer(http.HandlerFunc(f.serve))
		DeferCleanup(server.Close)
		client := NewClient(context.Background(), server.URL, nil, false)
		DeferCleanup(client.Close)
		Expect(client.Connect()).To(Succeed())
		_, err := client.Discover()
		Expect(err).To(HaveOccurred())
		Expect(f.calls).To(BeZero())
	})
	It("detects repeated discovery cursors", func() {
		f := fixture()
		f.list = `{"tools":[],"nextCursor":"same"}`
		server := httptest.NewServer(http.HandlerFunc(f.serve))
		DeferCleanup(server.Close)
		client := NewClient(context.Background(), server.URL, nil, false)
		DeferCleanup(client.Close)
		Expect(client.Connect()).To(Succeed())
		_, err := client.Discover()
		Expect(err).To(MatchError(ContainSubstring("mcp_discovery_incomplete")))
	})
	It("classifies input_required including empty requests without continuation", func() {
		f := fixture()
		f.result = `{"resultType":"input_required","inputRequests":{},"requestState":"state"}`
		server := httptest.NewServer(http.HandlerFunc(f.serve))
		DeferCleanup(server.Close)
		client := NewClient(context.Background(), server.URL, nil, false)
		DeferCleanup(client.Close)
		Expect(client.Connect()).To(Succeed())
		_, err := client.Discover()
		Expect(err).NotTo(HaveOccurred())
		result, err := client.Call("echo", map[string]any{})
		Expect(err).To(MatchError(ContainSubstring("mcp_capability_unsupported")))
		Expect(result).To(HaveKeyWithValue("requestState", "state"))
		Expect(f.calls).To(Equal(1))
	})
	It("bounds malformed and oversized responses", func() {
		f := fixture()
		f.result = `{"content":[{"type":"text","text":"` + strings.Repeat("x", MaxResponseBytes) + `"}]}`
		server := httptest.NewServer(http.HandlerFunc(f.serve))
		DeferCleanup(server.Close)
		client := NewClient(context.Background(), server.URL, nil, false)
		DeferCleanup(client.Close)
		Expect(client.Connect()).To(Succeed())
		_, err := client.Discover()
		Expect(err).NotTo(HaveOccurred())
		_, err = client.Call("echo", map[string]any{})
		Expect(err).To(MatchError(ContainSubstring("mcp_response_too_large")))
		Expect(client.CallAttempted()).To(BeTrue())
		Expect(f.calls).To(Equal(1))
	})
})
