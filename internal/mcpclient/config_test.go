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
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MCP configuration", func() {
	var dir string
	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", dir)
	})
	write := func(body string) string {
		path := filepath.Join(dir, "mcp.json")
		Expect(os.WriteFile(path, []byte(body), 0o600)).To(Succeed())
		return path
	}
	It("selects one file without reading even a corrupt active context", func() {
		Expect(os.WriteFile(filepath.Join(dir, "current"), []byte("../broken"), 0o600)).To(Succeed())
		path := write(`{"mcpServers":{"test":{"type":"http","url":"https://example.com/path/?q=1"}}}`)
		selected, err := LoadServer("", "test")
		Expect(err).NotTo(HaveOccurred())
		Expect(selected.ConfigFile).To(Equal(path))
		Expect(selected.URL).To(Equal("https://example.com/path/?q=1"))
		headers, err := selected.ResolveHeaders()
		Expect(err).NotTo(HaveOccurred())
		Expect(headers).To(BeEmpty())
		_, err = LoadServer(filepath.Join(dir, "missing.json"), "test")
		Expect(err).To(MatchError(ContainSubstring("mcp_config_not_found")))
	})
	It("keeps configured header values literal without changing the file", func() {
		path := write(
			`{"mcpServers":{"test":{"type":"http","url":"https://example.com","headers":{"X-Key":"${MCP_TEST_SECRET}"}},"other":{"type":"http","url":"https://example.net","headers":{"Authorization":"invalid credential"}}}}`,
		)
		GinkgoT().Setenv("MCP_TEST_SECRET", "secret")
		selected, err := LoadServer(path, "test")
		Expect(err).NotTo(HaveOccurred())
		headers, err := selected.ResolveHeaders()
		Expect(err).NotTo(HaveOccurred())
		Expect(headers.Get("X-Key")).To(Equal("${MCP_TEST_SECRET}"))
		Expect(selected.MaskedHeaders()).To(Equal(map[string]string{"X-Key": "***"}))
		content, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(ContainSubstring("${MCP_TEST_SECRET}"))
	})

	DescribeTable(
		"rejects invalid structure without reflecting secrets",
		func(body string) {
			_, err := LoadServer(write(body), "test")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid_mcp_config"))
			Expect(err.Error()).NotTo(ContainSubstring("SECRET"))
		},
		Entry("bad JSON", `{"SECRET`),
		Entry("null", `null`),
		Entry("missing servers", `{}`),
		Entry("unknown field", `{"mcpServers":{},"SECRET":true}`),
		Entry("wrong format", `{"servers":{}}`),
		Entry("duplicate keys", `{"mcpServers":{},"mcpServers":{}}`),
		Entry("duplicate server", `{"mcpServers":{"test":{},"test":{}}}`),
		Entry("null server", `{"mcpServers":{"test":null}}`),
		Entry(
			"null headers",
			`{"mcpServers":{"test":{"type":"http","url":"https://example.com","headers":null}}}`,
		),
		Entry(
			"unknown server field",
			`{"mcpServers":{"test":{"type":"http","url":"https://example.com","SECRET":true}}}`,
		),
		Entry("unsupported transport", `{"mcpServers":{"test":{"type":"stdio","url":"https://example.com"}}}`),
		Entry("non-http", `{"mcpServers":{"test":{"type":"http","url":"file:///SECRET"}}}`),
		Entry("userinfo", `{"mcpServers":{"test":{"type":"http","url":"https://user:SECRET@example.com"}}}`),
		Entry("fragment", `{"mcpServers":{"test":{"type":"http","url":"https://example.com/#SECRET"}}}`),
		Entry("invalid name", `{"mcpServers":{"-SECRET":{"type":"http","url":"https://example.com"}}}`),
		Entry(
			"header aliases",
			`{"mcpServers":{"test":{"type":"http","url":"https://example.com","headers":{"X-Key":"SECRET","x-key":"SECRET"}}}}`,
		),
		Entry(
			"managed header",
			`{"mcpServers":{"test":{"type":"http","url":"https://example.com","headers":{"Mcp-Session-Id":"SECRET"}}}}`,
		),
		Entry(
			"invalid header name",
			`{"mcpServers":{"test":{"type":"http","url":"https://example.com","headers":{"SECRET\n":"x"}}}}`,
		),
		Entry(
			"invalid header value",
			`{"mcpServers":{"test":{"type":"http","url":"https://example.com","headers":{"X-Key":"SECRET\n"}}}}`,
		),
	)
	DescribeTable("identifies the invalid server and field without exposing values", func(entry, field string) {
		_, err := LoadConfig(write(`{"mcpServers":{"broken":` + entry + `}}`))
		Expect(
			err,
		).To(
			MatchError(
				And(
					ContainSubstring("invalid_mcp_config"),
					ContainSubstring(`"broken"`),
					ContainSubstring(field),
				),
			),
		)
		Expect(err.Error()).NotTo(ContainSubstring("SECRET"))
	}, Entry("missing transport", `{"url":"https://example.com"}`, "type"),
		Entry("stdio", `{"type":"stdio","url":"https://example.com"}`, "type"),
		Entry("bad URL", `{"type":"http","url":"SECRET"}`, "url"),
		Entry("bad headers", `{"type":"http","url":"https://example.com","headers":{"X-Key":42}}`, "headers"))
	It("uses explicit files, exact names, and safe errors", func() {
		path := write(`{"mcpServers":{"Case":{"type":"http","url":"http://example.com"}}}`)
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", filepath.Join(dir, "unused"))
		_, err := LoadServer(path, "Case")
		Expect(err).NotTo(HaveOccurred())
		_, err = LoadServer(path, "case")
		Expect(err).To(MatchError(ContainSubstring("mcp_server_not_found")))
		_, err = LoadServer(dir, "test")
		Expect(err).To(MatchError(ContainSubstring("invalid_mcp_config")))
	})
	It("bounds configuration input", func() {
		_, err := LoadServer(write(strings.Repeat(" ", MaxInputBytes+1)), "test")
		Expect(err).To(MatchError(ContainSubstring("invalid_mcp_config")))
	})
	It("uses the home directory when the config directory override is empty", func() {
		GinkgoT().Setenv("HOME", dir)
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", "")
		Expect(os.Mkdir(filepath.Join(dir, ".bk-cli"), 0o700)).To(Succeed())
		path := filepath.Join(dir, ".bk-cli", "mcp.json")
		Expect(
			os.WriteFile(
				path,
				[]byte(`{"mcpServers":{"test":{"type":"http","url":"http://example.com"}}}`),
				0o600,
			),
		).To(
			Succeed(),
		)
		server, err := LoadServer("", "test")
		Expect(err).NotTo(HaveOccurred())
		Expect(server.ConfigFile).To(Equal(path))
	})
})

var _ = Describe("Server header resolution", func() {
	DescribeTable(
		"rejects managed or repeated headers even on a constructed server",
		func(headers map[string]string) {
			_, err := (Server{Headers: headers}).ResolveHeaders()
			Expect(err).To(HaveOccurred())
		},
		Entry("Host", map[string]string{"Host": "other.example"}),
		Entry("Connection", map[string]string{"Connection": "keep-alive"}),
		Entry("duplicates", map[string]string{"X-Key": "a", "x-key": "b"}),
	)
	DescribeTable(
		"validates authentication shapes",
		func(headers map[string]string, code string) {
			server := Server{Headers: headers}
			resolved, err := server.ResolveHeaders()
			if code != "" {
				Expect(err).To(MatchError(ContainSubstring(code)))
				Expect(err.Error()).NotTo(ContainSubstring("SECRET"))
			} else {
				Expect(err).NotTo(HaveOccurred())
				for name, value := range headers {
					Expect(resolved.Get(name)).To(Equal(value))
				}
			}
		},
		Entry("anonymous", map[string]string{}, ""),
		Entry("personal", map[string]string{"authorization": "Bearer SECRET"}, ""),
		Entry("custom scheme", map[string]string{"Authorization": "Basic SECRET", "X-API-Key": "SECRET"}, ""),
		Entry(
			"app ticket",
			map[string]string{
				"X-Bkapi-Authorization": `{"bk_app_code":"a","bk_app_secret":"SECRET","bk_ticket":"t"}`,
			},
			"",
		),
		Entry(
			"app token",
			map[string]string{
				"X-Bkapi-Authorization": `{"bk_app_code":"a","bk_app_secret":"SECRET","bk_token":"t"}`,
			},
			"",
		),
		Entry("access token", map[string]string{"X-Bkapi-Authorization": `{"access_token":"SECRET"}`}, ""),
		Entry(
			"conflict",
			map[string]string{
				"Authorization":         "Bearer SECRET",
				"x-bkapi-authorization": `{"access_token":"SECRET"}`,
			},
			"mcp_auth_conflict",
		),
		Entry("empty", map[string]string{"Authorization": ""}, "invalid_mcp_auth"),
		Entry("empty bearer", map[string]string{"Authorization": "bEaReR "}, "invalid_mcp_auth"),
		Entry(
			"bearer with spaces",
			map[string]string{"Authorization": "Bearer SECRET bad"},
			"invalid_mcp_auth",
		),
		Entry("non-object", map[string]string{"X-Bkapi-Authorization": `[]`}, "invalid_mcp_auth"),
		Entry("invalid JSON", map[string]string{"X-Bkapi-Authorization": `SECRET`}, "invalid_mcp_auth"),
		Entry("empty object", map[string]string{"X-Bkapi-Authorization": `{}`}, "invalid_mcp_auth"),
		Entry(
			"wrong type",
			map[string]string{"X-Bkapi-Authorization": `{"access_token":42}`},
			"invalid_mcp_auth",
		),
		Entry(
			"empty token",
			map[string]string{"X-Bkapi-Authorization": `{"access_token":" "}`},
			"invalid_mcp_auth",
		),
		Entry(
			"mixed modes",
			map[string]string{"X-Bkapi-Authorization": `{"access_token":"SECRET","bk_app_code":"a"}`},
			"invalid_mcp_auth",
		),
		Entry(
			"unknown field",
			map[string]string{"X-Bkapi-Authorization": `{"access_token":"SECRET","extra":"x"}`},
			"invalid_mcp_auth",
		),
		Entry(
			"two user tokens",
			map[string]string{
				"X-Bkapi-Authorization": `{"bk_app_code":"a","bk_app_secret":"SECRET","bk_ticket":"t","bk_token":"t"}`,
			},
			"invalid_mcp_auth",
		),
		Entry(
			"missing secret",
			map[string]string{"X-Bkapi-Authorization": `{"bk_app_code":"a","bk_ticket":"t"}`},
			"invalid_mcp_auth",
		),
	)
	DescribeTable("preserves template-like text literally", func(value string) {
		GinkgoT().Setenv("MCP_TEST_SECRET", "SECRET")
		GinkgoT().Setenv("MCP_TEST_BAD", "SECRET\r\n")
		GinkgoT().Setenv("BK_CLI_PERSONAL_TOKEN", "unrelated")
		headers, err := (Server{Headers: map[string]string{"X-Key": value}}).ResolveHeaders()
		Expect(err).NotTo(HaveOccurred())
		Expect(headers.Get("X-Key")).To(Equal(value))
		Expect(headers.Get("Authorization")).To(BeEmpty())
	}, Entry("existing variable", "${MCP_TEST_SECRET}"),
		Entry("missing variable", "${MCP_VARIABLE_NOT_CONFIGURED_42}"),
		Entry("escape", "$${MCP_TEST_SECRET}"),
		Entry("control characters in environment", "${MCP_TEST_BAD}"),
		Entry("unterminated", "${SECRET"), Entry("client syntax", "${env:SECRET}"),
		Entry("default", "${SECRET:-default}"), Entry("empty", "${}"),
		Entry("shell text", "$(whoami)/`whoami`"))
	It("does not turn a templated bearer value into a valid credential", func() {
		GinkgoT().Setenv("MCP_TEST_SECRET", "valid-token")
		_, err := (Server{Headers: map[string]string{"Authorization": "Bearer ${MCP_TEST_SECRET}"}}).ResolveHeaders()
		Expect(err).To(MatchError(ContainSubstring("invalid_mcp_auth")))
	})
})
