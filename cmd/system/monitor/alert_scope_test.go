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

// Package monitorapi calls individual Monitor HTTP APIs through the shared request executor.
// The server remains the source of truth for tool contracts, resource ownership and permissions.
package monitor

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

var _ = Describe("native alert business scope", func() {
	var dir string
	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", dir)
		GinkgoT().Setenv("BK_CLI_MONITOR_PROFILE_FILE", "")
	})

	DescribeTable(
		"derives native scope after body, flags and Profile selection",
		func(tool, body, biz string, profile bool) {
			Expect(systemtest.SetupTestContext("https://gateway.example.test/api")).To(Succeed())
			cmd := toolCommand(tool, true)
			setFlags(cmd, "since", "1h")
			if tool == "get_alert_top_n" {
				setFlags(cmd, "fields", "severity")
			}
			if body != "" {
				setFlags(cmd, "body", body)
			}
			if biz != "" {
				setFlags(cmd, "bk_biz_id", biz)
			}
			if profile {
				file := filepath.Join(dir, "profile.yaml")
				Expect(os.WriteFile(file, []byte(testProfiles), 0o600)).To(Succeed())
				setFlags(cmd, "profile", "worker", "profile_file", file)
			}
			args := dryRunArgs(cmd)
			expected := "100100"
			if biz == "9007199254740993" || body == `{"bk_biz_id":9007199254740993}` {
				expected = "9007199254740993"
			}
			Expect(args["bk_biz_ids"]).To(Equal([]any{expected}))
		},
		Entry("named override", "list_alerts", `{"bk_biz_id":"900001"}`, "100100", false),
		Entry("matching raw array", "list_alerts", `{"bk_biz_id":"100100","bk_biz_ids":["100100"]}`, "", false),
		Entry(
			"numeric matching array",
			"list_alerts",
			`{"bk_biz_id":"100100","bk_biz_ids":[100100]}`,
			"",
			false,
		),
		Entry("large named identifier", "list_alerts", "", "9007199254740993", false),
		Entry("large body identifier", "list_alerts", `{"bk_biz_id":9007199254740993}`, "", false),
		Entry("Profile scope", "list_alerts", "", "", true),
		Entry("TopN scope", "get_alert_top_n", "", "100100", false),
	)

	DescribeTable("rejects conflicting backend scope before making a request",
		func(raw string) {
			var calls atomic.Int32
			server := httptest.NewServer(
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }),
			)
			DeferCleanup(server.Close)
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			cmd := toolCommand("list_alerts", false)
			setFlags(cmd, "body", `{"bk_biz_id":"100100","bk_biz_ids":`+raw+`}`, "since", "1h")
			_, err := runProfileCommand(cmd)
			Expect(err).To(HaveOccurred())
			Expect(err.(*output.CLIError).Code).To(Equal("invalid_arguments"))
			Expect(calls.Load()).To(BeZero())
		},
		Entry("foreign business", `["100200"]`),
		Entry("multiple businesses", `["100100","100200"]`),
		Entry("duplicates", `["100100","100100"]`),
		Entry("empty array", `[]`),
		Entry("scalar", `"100100"`),
		Entry("null", `null`),
	)

	It("derives scope from a V2 binding and preserves its TopN preset", func() {
		Expect(systemtest.SetupTestContext("https://gateway.example.test/api")).To(Succeed())
		file := filepath.Join(dir, "v2.yaml")
		raw := `version: 2
sources: {sample-source: {context: default, bk_biz_id: "100100"}}
profiles:
  sample:
    components:
      api:
        bindings:
          alerts:
            module: alert
            source: sample-source
            tools: {get_alert_top_n: {fields: [severity]}}
`
		Expect(os.WriteFile(file, []byte(raw), 0o600)).To(Succeed())
		for _, tool := range []string{"list_alerts", "get_alert_top_n"} {
			cmd := toolCommand(tool, true)
			setFlags(cmd, "profile", "sample", "profile_file", file, "component", "api", "since", "1h")
			args := dryRunArgs(cmd)
			Expect(args["bk_biz_ids"]).To(Equal([]any{"100100"}))
			if tool == "get_alert_top_n" {
				Expect(args["fields"]).To(Equal([]any{"severity"}))
			}
		}
	})

	It("sends the derived scope and follows a same-business list ID to detail", func() {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			calls.Add(1)
			if r.Method == http.MethodPost {
				Expect(r.URL.Path).To(HaveSuffix("/mcp/list_alerts/"))
				args := readMonitorRequest(r)
				Expect(args["bk_biz_ids"]).To(Equal([]any{"100100"}))
				_, _ = io.WriteString(
					w,
					`{"result":true,"data":{"alerts":[{"id":"sample-alert","bk_biz_id":100100}]}}`,
				)
			} else {
				Expect(r.URL.Path).To(HaveSuffix("/mcp/get_alert_info/"))
				Expect(r.URL.Query().Get("bk_biz_id")).To(Equal("100100"))
				Expect(r.URL.Query().Get("id")).To(Equal("sample-alert"))
				_, _ = io.WriteString(w, `{"result":true,"data":{"id":"sample-alert"}}`)
			}
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		cmd := toolCommand("list_alerts", false)
		setFlags(cmd, "bk_biz_id", "100100", "since", "1h")
		env, err := runProfileCommand(cmd)
		Expect(err).NotTo(HaveOccurred())
		row := env["data"].(map[string]any)["alerts"].([]any)[0].(map[string]any)
		cmd = toolCommand("get_alert_info", false)
		setFlags(cmd, "bk_biz_id", "100100", "id", row["id"].(string))
		_, err = runProfileCommand(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(calls.Load()).To(Equal(int32(2)))
	})

	DescribeTable(
		"withholds malformed or out-of-scope responses before jq",
		func(data, code string) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Bkapi-Request-Id", "scope-request")
				_, _ = io.WriteString(w, `{"result":true,"data":`+data+`}`)
			}))
			DeferCleanup(server.Close)
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			cmd := toolCommand("list_alerts", false)
			setFlags(cmd, "bk_biz_id", "100100", "since", "1h", "jq", ".data")
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			err := cmd.RunE(cmd, nil)
			Expect(err.(*output.CLIError).Code).To(Equal(code))
			Expect(stdout.String()).To(BeEmpty())
			Expect(stderr.String()).NotTo(ContainSubstring("PRIVATE-CONTENT"))
			var failure map[string]any
			Expect(json.Unmarshal(stderr.Bytes(), &failure)).To(Succeed())
			Expect(failure["data"]).To(HaveKeyWithValue("request_id", "scope-request"))
		},
		Entry(
			"foreign business",
			`{"alerts":[{"bk_biz_id":"100200","description":"PRIVATE-CONTENT"}]}`,
			"response_scope_mismatch",
		),
		Entry("missing business", `{"alerts":[{"description":"PRIVATE-CONTENT"}]}`, "response_scope_mismatch"),
		Entry("missing list", `{}`, "invalid_response"),
		Entry("invalid list", `{"alerts":null}`, "invalid_response"),
	)
})
