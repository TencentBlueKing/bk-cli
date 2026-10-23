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

package monitor

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	json "github.com/goccy/go-json"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
)

var _ = Describe("reviewed Monitor request contracts", func() {
	var dir string
	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", dir)
		GinkgoT().Setenv("BK_CLI_MONITOR_PROFILE_FILE", "")
	})

	It("sends a log command directly with invocation stage, headers and tenant", func() {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			calls.Add(1)
			Expect(r.Method).To(Equal(http.MethodPost))
			Expect(r.URL.Path).To(Equal("/bk-monitor/testing/mcp/search_logs/"))
			Expect(r.Header.Get("X-Example")).To(Equal("probe"))
			Expect(r.Header.Get("X-Bkapi-Authorization")).NotTo(BeEmpty())
			Expect(r.Header.Get("X-Bk-Tenant-Id")).To(Equal("tenant-test"))
			args := readMonitorRequest(r)
			Expect(args).To(HaveKeyWithValue("bk_biz_id", "100100"))
			Expect(args).NotTo(HaveKey("tool_name"))
			Expect(args).NotTo(HaveKey("tool_args"))
			_, err := io.WriteString(w, `{"result":true,"data":{"list":[]}}`)
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		cmd := toolCommand("search_logs", false)
		setFlags(
			cmd,
			"bk_biz_id",
			"100100",
			"index_set_id",
			"101001",
			"since",
			"15m",
			"stage",
			"testing",
			"header",
			"X-Example:probe",
			"header",
			"X-Bk-Tenant-Id:tenant-test",
			"jq",
			".data.list",
		)
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		Expect(cmd.RunE(cmd, nil)).To(Succeed())
		Expect(stdout.String()).To(Equal("[]\n"))
		Expect(calls.Load()).To(Equal(int32(1)))
	})

	DescribeTable(
		"projects the complete final envelope including Profile provenance",
		func(dry bool, expr, expected string) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				calls.Add(1)
				Expect(r.Method).To(Equal(http.MethodGet))
				Expect(r.URL.Path).To(Equal("/bk-monitor/prod/mcp/get_index_set_fields/"))
				Expect(r.URL.Query().Get("index_set_id")).To(Equal("101003"))
				_, err := io.WriteString(w, `{"result":true,"data":{"fields":[]}}`)
				Expect(err).NotTo(HaveOccurred())
			}))
			DeferCleanup(server.Close)
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			file := filepath.Join(dir, "profiles.yaml")
			Expect(os.WriteFile(file, []byte(testProfiles), 0o600)).To(Succeed())
			cmd := toolCommand("get_index_set_fields", dry)
			setFlags(cmd, "profile", "worker", "profile_file", file, "jq", expr)
			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			Expect(cmd.RunE(cmd, nil)).To(Succeed())
			Expect(stdout.String()).To(Equal(expected))
			if dry {
				Expect(calls.Load()).To(BeZero())
			} else {
				Expect(calls.Load()).To(Equal(int32(1)))
			}
		},
		Entry(
			"native result",
			false,
			`{profile:.profile.name,fields:.data.fields}`,
			"{\"fields\":[],\"profile\":\"worker\"}\n",
		),
		Entry(
			"redacted preview",
			true,
			`{profile:.profile.name,index:.request.params.index_set_id,auth:.request.headers["X-Bkapi-Authorization"]}`,
			"{\"auth\":\"{...redacted...}\",\"index\":101003,\"profile\":\"worker\"}\n",
		),
	)

	It("retains Profile provenance and the completed result when jq execution fails", func() {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			_, _ = io.WriteString(w, `{"result":true,"data":{"fields":[]}}`)
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		file := filepath.Join(dir, "profiles.yaml")
		Expect(os.WriteFile(file, []byte(testProfiles), 0o600)).To(Succeed())
		cmd := toolCommand("get_index_set_fields", false)
		setFlags(cmd, "profile", "worker", "profile_file", file, "jq", `.profile.name | .missing[]`)
		var stdout, stderr bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		Expect(cmd.RunE(cmd, nil)).NotTo(Succeed())
		Expect(calls.Load()).To(Equal(int32(1)))
		var env map[string]any
		Expect(json.Unmarshal(stdout.Bytes(), &env)).To(Succeed())
		Expect(env["profile"].(map[string]any)["name"]).To(Equal("worker"))
		Expect(env["data"]).To(HaveKey("fields"))
		Expect(stderr.String()).To(ContainSubstring("jq_execution_error"))
		Expect(stderr.String()).To(ContainSubstring("request has already completed"))
	})

	It("rejects invalid jq before context, body input or network access", func() {
		cmd := toolCommand("search_logs", false)
		cmd.SetArgs([]string{"--body", "-", "--jq", ".["})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		Expect(cmd.Execute()).To(MatchError(ContainSubstring("--jq")))
	})

	DescribeTable(
		"checks the resource resolved for the original tool, with options on every intermediate request",
		func(table string, valid bool) {
			tables := []string{}
			sqlCalls := 0
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				calls++
				Expect(r.URL.Path).To(HavePrefix("/bk-monitor/testing/mcp/"))
				Expect(r.Header.Get("X-Example")).To(Equal("preflight"))
				op := facade(r.URL.Path)
				args := readMonitorRequest(r)
				data := map[string]any{}
				switch op {
				case "lookup_tool":
					data["tools"] = []any{
						map[string]any{
							"name":                  args["tool_name"],
							"risk":                  "query",
							"requires_confirmation": false,
						},
					}
				case "lookup_tool_schema":
					data["catalog_version"] = catalogData.Version
					data["permission"] = map[string]any{
						"fallback_permission": map[string]any{
							"resource_arg": "bk_biz_id",
							"target_arg":   "table_id",
						},
					}
				case "lookup_permissions":
					data["authorized"] = true
					data["scopes"] = []any{
						map[string]any{"tool_name": args["tool_name"], "state": "granted"},
					}
				case "list_time_series_metrics":
					Expect(r.Method).To(Equal(http.MethodGet))
					tables = append(tables, r.URL.Query().Get("table_id"))
					if !valid {
						_, _ = io.WriteString(
							w,
							`{"result":false,"message":"table does not exist"}`,
						)
						return
					}
					data["fields"] = []any{}
				case "execute_sql_query":
					sqlCalls++
					Fail("profile check must not execute SQL")
				default:
					Fail("unexpected preflight API: " + op)
				}
				Expect(
					json.NewEncoder(w).Encode(map[string]any{"result": true, "data": data}),
				).To(
					Succeed(),
				)
			}))
			DeferCleanup(server.Close)
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			profile := `version: 2
sources: {sample-source: {context: default, bk_biz_id: "100100"}}
profiles:
  sample:
    components:
      api:
        bindings:
          metrics:
            module: metric
            source: sample-source
            args: {table_id: existing.table}
            tools:
              execute_sql_query: {table_id: TABLE, sql: 'SELECT 1'}
`
			file := filepath.Join(dir, "profiles.yaml")
			Expect(
				os.WriteFile(file, []byte(strings.ReplaceAll(profile, "TABLE", table)), 0o600),
			).To(
				Succeed(),
			)
			cmd := newProfileCheckCmd(systemtest.BuildDeps(false))
			setFlags(
				cmd,
				"profile",
				"sample",
				"profile_file",
				file,
				"stage",
				"testing",
				"header",
				"X-Example:preflight",
			)
			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			err := cmd.RunE(cmd, nil)
			if valid {
				Expect(err).NotTo(HaveOccurred())
				Expect(stdout.String()).To(ContainSubstring(`"validation": "metadata"`))
			} else {
				Expect(err).To(HaveOccurred())
				Expect(stdout.String()).To(ContainSubstring("resource_not_found"))
			}
			Expect(tables).To(Equal([]string{table}))
			Expect(sqlCalls).To(BeZero())
			Expect(calls).To(BeNumerically(">", 3))
		},
		Entry("overridden missing table", "missing.table", false),
		Entry("overridden visible table", "override.table", true),
	)
})
