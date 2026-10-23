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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	json "github.com/goccy/go-json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/cmd/system/internal/monitorapi"
	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

var _ = Describe("monitor reliability boundaries", func() {
	var dir string
	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", dir)
		GinkgoT().Setenv("BK_CLI_MONITOR_PROFILE_FILE", "")
	})

	It("constructs a dry-run request for every catalog tool without any network call", func() {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		for _, item := range catalogData.Tools {
			cmd := toolCommand(item.Name, true)
			window := hasParam(item, "start_time") && hasParam(item, "end_time")
			for _, p := range item.Params {
				if !p.Required || (window && (p.Name == "start_time" || p.Name == "end_time")) {
					continue
				}
				value := "sample"
				if len(p.Enum) > 0 {
					value = p.Enum[0]
				}
				switch p.Type {
				case "integer", "number":
					value = "1"
				case "boolean":
					value = "true"
				case "object":
					value = "{}"
				case "array":
					if !isScalar(p.Items) {
						value = "[]"
					}
				}
				if p.Name == "bk_biz_id" {
					value = "100100"
				}
				if p.TimeUnit != "" {
					value = "now"
				}
				setFlags(cmd, p.Name, value)
			}
			if item.Name == "search_logs" {
				setFlags(cmd, "index_set_id", "101003")
			}
			if window {
				setFlags(cmd, "since", "15m")
			}
			env, err := runProfileCommand(cmd)
			Expect(err).NotTo(HaveOccurred(), item.Name)
			Expect(env["dry_run"]).To(BeTrue(), item.Name)
			request := env["request"].(map[string]any)
			body := previewArgs(env)
			Expect(request["method"]).To(Equal(item.Endpoint.Method))
			Expect(request["url"]).To(HaveSuffix(item.Endpoint.Path))
			Expect(body).NotTo(HaveKey("tool_name"))
			Expect(body).NotTo(HaveKey("tool_args"))
			for _, p := range item.Params {
				if p.Required {
					Expect(body).To(HaveKey(p.Name), item.Name)
				}
			}
		}
		Expect(calls).To(BeZero())
	})

	It("preserves the actual denied index in the application hint and excludes query text", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"result":false,"code":403,"detail":{"state":"missing"}}`))
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		profileFile := filepath.Join(dir, "profiles.yaml")
		Expect(os.WriteFile(profileFile, []byte(testProfiles), 0o600)).To(Succeed())
		cmd := toolCommand("search_logs", false)
		setFlags(
			cmd,
			"profile",
			"worker",
			"profile_file",
			profileFile,
			"index_set_id",
			"222",
			"query_string",
			"DO-NOT-ECHO",
			"since",
			"15m",
		)
		stderr := &bytes.Buffer{}
		cmd.SetErr(stderr)
		err := cmd.RunE(cmd, nil)
		Expect(err.(*output.CLIError).Code).To(Equal("permission_missing"))
		var env map[string]any
		Expect(json.Unmarshal(stderr.Bytes(), &env)).To(Succeed())
		next := env["data"].(map[string]any)["next"].([]any)[0].(string)
		Expect(next).To(ContainSubstring(`"index_set_id":222`))
		Expect(next).To(ContainSubstring("--context default"))
		Expect(next).To(ContainSubstring("--profile worker"))
		Expect(next).NotTo(ContainSubstring("DO-NOT-ECHO"))
	})

	It("returns executable scoped hints for permission lists and metadata errors", func() {
		p := &selectedProfile{
			Name:    "worker",
			File:    "profiles.yaml",
			Profile: queryProfile{Context: "default", BizID: "100100"},
		}
		row := permissionRow("get_index_set_fields", "requires_resource", "100100", "default", p)
		Expect(row["next"]).NotTo(ContainSubstring("<resource json>"))
		Expect(row["next"]).To(ContainSubstring("--tool get_index_set_fields"))
		for _, code := range []string{"resource_blocked", "resource_not_found", "retry_later"} {
			next := profileDiagnosticNext(map[string]any{"decision": code}, "list_time_series_metrics", p)
			Expect(strings.Join(next, " ")).NotTo(ContainSubstring("permission apply"))
			Expect(strings.Join(next, " ")).NotTo(ContainSubstring("permission check"))
		}
		decision := monitorapi.InputError("permission_missing", "denied")
		err := preservePermissionTarget(
			decision,
			"default",
			"get_index_set_fields",
			"100100",
			map[string]any{"index_set_id": 101003},
		)
		Expect(err).To(Equal(decision))
		Expect(decision.Next[0]).To(ContainSubstring(`"index_set_id":101003`))
	})

	It("accepts exact string indices and saved Profile metadata for the same scope", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		file := filepath.Join(dir, "anchor.json")
		Expect(
			os.WriteFile(
				file,
				[]byte(
					`{"ok":true,"profile":{"context":"default","bk_biz_id":"100100","resolved_parameters":{"index_set_id":"101003"}},"data":{"list":[{"dtEventTimeStamp":"1","serverIp":"ip","gseIndex":9007199254740993}]}}`,
				),
				0o600,
			),
		).To(
			Succeed(),
		)
		cmd := toolCommand("search_index_set_context", true)
		setFlags(cmd, "bk_biz_id", "100100", "from", file)
		args := dryRunArgs(cmd)
		Expect(args["index_set_id"]).To(BeNumerically("==", 101003))
		Expect(args["gseIndex"]).To(Equal("9007199254740993"))
	})

	DescribeTable(
		"checks each relation item instead of trusting the outer success",
		func(payload, decision string) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(
					[]byte(
						`{"result":true,"data":` + payload + `}`,
					),
				)
			}))
			DeferCleanup(server.Close)
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			cmd := toolCommand("find_relations", false)
			setFlags(cmd, "bk_biz_id", "100100", "query_list", `[{"timestamp":1,"source_info":{}}]`)
			stderr := &bytes.Buffer{}
			cmd.SetErr(stderr)
			_, err := systemtest.CaptureCommandStdout(func() error { return cmd.RunE(cmd, nil) })
			if decision == "" {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err.(*output.CLIError).Code).To(Equal(decision))
			}
			Expect(stderr.String()).NotTo(ContainSubstring("SECRET upstream"))
		},
		Entry("successful but no matching target", `{"data":[{"code":200,"target_list":[]}]}`, ""),
		Entry(
			"one failed item",
			`{"data":[{"code":400,"message":"SECRET upstream"}]}`,
			"relation_query_failed",
		),
		Entry("missing item", `{"data":[]}`, "partial_result"),
		Entry("missing per-item success code", `{"data":[{}]}`, "relation_query_failed"),
	)

	It("rejects trailing JSON instead of silently using only the first object", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		cmd := toolCommand("list_alerts", true)
		setFlags(cmd, "body", `{"bk_biz_id":"100100"} {"bk_biz_id":"200200"}`, "since", "15m")
		_, err := runProfileCommand(cmd)
		Expect(err).To(HaveOccurred())
		Expect(err.(*output.CLIError).Code).To(Equal("invalid_body"))
	})

	It("bounds inline bodies as well as file and stdin bodies", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		cmd := toolCommand("list_alerts", true)
		setFlags(
			cmd,
			"body",
			`{"bk_biz_id":"100100","query_string":"`+strings.Repeat("x", maxInputBytes)+`"}`,
			"since",
			"15m",
		)
		_, err := runProfileCommand(cmd)
		Expect(err).To(HaveOccurred())
		Expect(err.(*output.CLIError).Code).To(Equal("invalid_body"))
	})

	It("preserves large integers in structured named flags", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		cmd := toolCommand("list_alerts", true)
		setFlags(
			cmd,
			"bk_biz_id",
			"100100",
			"conditions",
			`[{"key":"id","value":[9007199254740993]}]`,
			"since",
			"15m",
		)
		stdout, err := systemtest.CaptureCommandStdout(func() error { return cmd.RunE(cmd, nil) })
		Expect(err).NotTo(HaveOccurred())
		Expect(stdout).To(ContainSubstring("9007199254740993"))
	})

	DescribeTable(
		"rejects unusable saved anchors",
		func(raw, code string) {
			Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
			file := filepath.Join(dir, "anchor.json")
			Expect(os.WriteFile(file, []byte(raw), 0o600)).To(Succeed())
			cmd := toolCommand("search_index_set_context", true)
			setFlags(cmd, "bk_biz_id", "100100", "index_set_id", "101003", "from", file)
			_, err := runProfileCommand(cmd)
			Expect(err).To(HaveOccurred())
			Expect(err.(*output.CLIError).Code).To(Equal(code))
		},
		Entry(
			"nested mock",
			`{"ok":true,"data":{"list":[{"__demo_mock__":true,"__index_set_id__":101003,"dtEventTimeStamp":"1","serverIp":"ip","gseIndex":"2"}]}}`,
			"mock_record",
		),
		Entry(
			"failed envelope",
			`{"ok":false,"data":{"list":[{"__index_set_id__":101003,"dtEventTimeStamp":"1","serverIp":"ip","gseIndex":"2"}]}}`,
			"invalid_from",
		),
		Entry("trailing data", `{"dtEventTimeStamp":"1","serverIp":"ip","gseIndex":"2"} {}`, "invalid_from"),
		Entry(
			"invalid index must not be ignored",
			`{"__index_set_id__":"bad","dtEventTimeStamp":"1","serverIp":"ip","gseIndex":"2"}`,
			"invalid_anchor_index",
		),
		Entry(
			"saved in another context",
			`{"ok":true,"profile":{"context":"other","bk_biz_id":"100100"},"data":{"list":[{"__index_set_id__":101003,"dtEventTimeStamp":"1","serverIp":"ip","gseIndex":"2"}]}}`,
			"anchor_context_mismatch",
		),
		Entry(
			"saved from another business",
			`{"ok":true,"profile":{"context":"default","bk_biz_id":"200200"},"data":{"list":[{"__index_set_id__":101003,"dtEventTimeStamp":"1","serverIp":"ip","gseIndex":"2"}]}}`,
			"anchor_business_mismatch",
		),
	)

	DescribeTable(
		"never treats a mismatched or inconsistent permission reply as ready",
		func(permission string) {
			calls := []string{}
			server := checkServer(&calls, permission)
			DeferCleanup(server.Close)
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			cmd := newPermissionCheckCmd(systemtest.BuildDeps(false))
			setFlags(
				cmd,
				"bk_biz_id",
				"100100",
				"tool",
				"get_index_set_fields",
				"body",
				`{"index_set_id":101003}`,
			)
			stderr := &bytes.Buffer{}
			cmd.SetErr(stderr)
			err := cmd.RunE(cmd, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.(*output.CLIError).Code).To(Equal("permission_response_invalid"))
			Expect(calls).NotTo(ContainElement("execute_tool"))
		},
		Entry("empty scopes", `{"result":true,"data":{"authorized":true,"scopes":[]}}`),
		Entry(
			"wrong tool",
			`{"result":true,"data":{"authorized":true,"scopes":[{"tool_name":"list_alerts","state":"granted"}]}}`,
		),
		Entry(
			"denied scope but granted summary",
			`{"result":true,"data":{"authorized":true,"scopes":[{"tool_name":"get_index_set_fields","state":"missing"}]}}`,
		),
		Entry(
			"duplicate scopes",
			`{"result":true,"data":{"authorized":true,"scopes":[{"tool_name":"get_index_set_fields","state":"granted"},{"tool_name":"get_index_set_fields","state":"granted"}]}}`,
		),
	)
})
