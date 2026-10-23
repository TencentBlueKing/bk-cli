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
	"strings"
	"time"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

var _ = Describe("monitor command tree", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-monitor-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
		now = func() time.Time { return time.Unix(1800000000, 0) }
	})

	AfterEach(func() {
		now = time.Now
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("exposes every reviewed read-only tool in exactly one module", func() {
		Expect(catalogErr).NotTo(HaveOccurred())
		Expect(catalogData.Tools).To(HaveLen(65))
		seen := map[string]bool{}
		for _, item := range catalogData.Tools {
			Expect(seen).NotTo(HaveKey(item.Name))
			seen[item.Name] = true
			Expect(commandName(item.Name)).To(Equal("bk-cli monitor " + item.Module + " " + item.Name))
		}
		Expect(seen).NotTo(HaveKey("create_dashboard"))
		Expect(seen).NotTo(HaveKey("get_log_extract_download_url"))
	})

	It("documents permission requirements for every module and tool", func() {
		for _, item := range catalogData.Tools {
			help := longHelp(item, false)
			if item.Permission.Mode == "exempt" {
				Expect(help).To(ContainSubstring("no business permission is required"))
				continue
			}
			Expect(item.Permission.MCP).NotTo(BeNil(), item.Name)
			Expect(item.Permission.Native).NotTo(BeNil(), item.Name)
			Expect(help).To(ContainSubstring("Permission (any one is enough):"), item.Name)
			Expect(
				help,
			).To(
				ContainSubstring(
					"bk-cli monitor permission apply --bk_biz_id <bk_biz_id> --tool "+item.Name,
				),
				item.Name,
			)
		}
		for _, item := range modules {
			Expect(
				modulePermissionHelp(item.Name),
			).To(
				ContainSubstring("Actions used by this module"),
				item.Name,
			)
		}
	})

	It("returns both apply links for a missing tool permission", func() {
		calls := []string{}
		server := checkServer(
			&calls,
			`{"result":true,"data":{"authorized":false,"scopes":[{"tool_name":"get_index_set_fields","state":"missing",`+
				`"legacy_permission":{"system_id":"bk_monitorv3","action_id":"using_log_mcp","resource_type":"space","apply_url":"https://iam.invalid/mcp"},`+
				`"fallback_permission":{"system_id":"bk_log_search","action_id":"search_log_v2","resource_type":"indices","apply_url":"https://iam.invalid/native"}}]}}`,
		)
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		cmd := newPermissionApplyCmd(systemtest.BuildDeps(false))
		setFlags(cmd, "bk_biz_id", "100100", "tool", "get_index_set_fields", "body", `{"index_set_id":123}`)

		stdout, err := systemtest.CaptureCommandStdout(func() error { return cmd.RunE(cmd, nil) })
		Expect(err).NotTo(HaveOccurred())
		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		data := env["data"].(map[string]any)
		Expect(data["decision"]).To(Equal("apply_required"))
		anyOf := data["applications"].([]any)[0].(map[string]any)["any_of"].([]any)
		Expect(anyOf).To(HaveLen(2))
		Expect(stdout).To(ContainSubstring("https://iam.invalid/mcp"))
		Expect(stdout).To(ContainSubstring("https://iam.invalid/native"))
		Expect(calls).NotTo(ContainElement("execute_tool"))
	})

	It("lists permission states without probing every resource", func() {
		calls := []string{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, facade(r.URL.Path))
			_, _ = w.Write([]byte(`{"result":true,"data":{"tools":[` +
				`{"name":"list_index_sets","permission_state":"granted"},` +
				`{"name":"get_index_set_fields","permission_state":"requires_resource"},` +
				`{"name":"search_logs","permission_state":"missing"}]}}`))
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		cmd := newPermissionListCmd(systemtest.BuildDeps(false))
		setFlags(cmd, "bk_biz_id", "100100", "module", "log")

		stdout, err := systemtest.CaptureCommandStdout(func() error { return cmd.RunE(cmd, nil) })
		Expect(err).NotTo(HaveOccurred())
		Expect(calls).To(Equal([]string{"lookup_tool"}))
		Expect(stdout).To(ContainSubstring(`"missing": 1`))
		Expect(
			stdout,
		).To(
			ContainSubstring("bk-cli monitor permission apply --bk_biz_id 100100 --tool search_logs"),
		)
	})

	It("builds search_logs from a scene, a recent window and repeatable fields", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		cmd := toolCommand("search_logs", true)
		setFlags(
			cmd,
			"bk_biz_id",
			"100100",
			"scene",
			"host",
			"where",
			"serverIp=192.0.2.10",
			"query_string",
			"ERROR",
			"since",
			"15m",
			"limit",
			"5",
			"keep_columns",
			"dtEventTimeStamp,serverIp",
		)

		args := dryRunArgs(cmd)
		Expect(args["target_type"]).To(Equal("scene"))
		Expect(args["start_time"]).To(Equal("1799999100"))
		Expect(args["end_time"]).To(Equal("1800000000"))
		Expect(args["limit"]).To(BeNumerically("==", 5))
		Expect(args["keep_columns"]).To(Equal([]any{"dtEventTimeStamp", "serverIp"}))
		Expect(args["table_id_conditions"]).To(HaveLen(1))
	})

	It("uses milliseconds when the tool contract asks for them", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		cmd := toolCommand("execute_sql_query", true)
		setFlags(cmd, "bk_biz_id", "100100", "table_id", "x.y", "sql", "select 1", "since", "1h")

		args := dryRunArgs(cmd)
		Expect(args["start_time"]).To(Equal("1799996400000"))
		Expect(args["end_time"]).To(Equal("1800000000000"))
	})

	It("merges --body and lets named flags override matching keys", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		cmd := toolCommand("list_alerts", true)
		setFlags(
			cmd,
			"body",
			`{"bk_biz_id":"1","page_size":"50","conditions":[{"key":"severity","value":[1]}]}`,
			"bk_biz_id",
			"100100",
			"status",
			"ABNORMAL",
			"since",
			"1h",
		)

		args := dryRunArgs(cmd)
		Expect(args["bk_biz_id"]).To(Equal("100100"))
		Expect(args["page_size"]).To(Equal("50"))
		Expect(args["status"]).To(Equal([]any{"ABNORMAL"}))
		Expect(args["conditions"]).To(HaveLen(1))
	})

	It("explains a missing log target before sending a request", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		cmd := toolCommand("search_logs", false)
		setFlags(cmd, "bk_biz_id", "100100", "since", "15m")
		stderr := &bytes.Buffer{}
		cmd.SetErr(stderr)

		err := cmd.RunE(cmd, nil)
		Expect(err.(*output.CLIError).Code).To(Equal("needs_resource"))
		Expect(stderr.String()).To(ContainSubstring("bk-cli monitor log list_log_scenes --bk_biz_id 100100"))
	})

	It("builds context from saved search output without changing anchor values", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		entry := tmpDir + "/hit.json"
		Expect(os.WriteFile(entry, []byte(`{"ok":true,"data":{"list":[{"__index_set_id__":101003,`+
			`"dtEventTimeStamp":1791600000000,"serverIp":"192.0.2.10","gseIndex":9007199254740993,"iterationIndex":0}]}}`), 0o600)).To(Succeed())
		cmd := toolCommand("search_index_set_context", true)
		setFlags(cmd, "bk_biz_id", "100100", "from", entry, "size", "2")

		args := dryRunArgs(cmd)
		Expect(args["index_set_id"]).To(BeNumerically("==", 101003))
		Expect(args["gseIndex"]).To(Equal("9007199254740993"))
		Expect(args["zero"]).To(BeTrue())
		Expect(args["begin"]).To(Equal("0"))
		Expect(args["size"]).To(Equal("2"))
	})

	It("rejects a context anchor from another index set", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		entry := tmpDir + "/hit.json"
		Expect(
			os.WriteFile(
				entry,
				[]byte(
					`{"__index_set_id__":101003,"dtEventTimeStamp":"1","serverIp":"192.0.2.10","gseIndex":"1"}`,
				),
				0o600,
			),
		).To(
			Succeed(),
		)
		cmd := toolCommand("search_index_set_context", true)
		setFlags(cmd, "bk_biz_id", "100100", "from", entry, "index_set_id", "568")
		cmd.SetErr(io.Discard)

		err := cmd.RunE(cmd, nil)
		Expect(err.(*output.CLIError).Code).To(Equal("anchor_index_mismatch"))
	})

	It("runs permission check as lookup_tool, lookup_tool_schema, then lookup_permissions", func() {
		calls := []string{}
		server := checkServer(
			&calls,
			`{"result":true,"data":{"authorized":false,"scopes":[{"tool_name":"get_index_set_fields","state":"requires_resource"}]}}`,
		)
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		cmd := newPermissionCheckCmd(systemtest.BuildDeps(false))
		setFlags(cmd, "bk_biz_id", "100100", "tool", "get_index_set_fields")
		stderr := &bytes.Buffer{}
		cmd.SetErr(stderr)

		err := cmd.RunE(cmd, nil)
		Expect(err.(*output.CLIError).Code).To(Equal("needs_resource"))
		Expect(calls).To(Equal([]string{"lookup_tool", "lookup_tool_schema", "lookup_permissions"}))
		Expect(stderr.String()).To(ContainSubstring("bk-cli monitor log list_index_sets --bk_biz_id 100100"))
	})

	It("prints ready without executing the data tool", func() {
		calls := []string{}
		server := checkServer(
			&calls,
			`{"result":true,"data":{"authorized":true,"scopes":[{"tool_name":"get_index_set_fields","state":"granted"}]}}`,
		)
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
			`{"index_set_id":3660,"query_string":"secret"}`,
		)

		stdout, err := systemtest.CaptureCommandStdout(func() error { return cmd.RunE(cmd, nil) })
		Expect(err).NotTo(HaveOccurred())
		Expect(stdout).To(ContainSubstring(`"decision": "ready"`))
		Expect(calls).NotTo(ContainElement("execute_tool"))
	})

	It("stops before permission lookup when the read-only contract changes", func() {
		calls := []string{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, facade(r.URL.Path))
			_, _ = w.Write(
				[]byte(
					`{"result":true,"data":{"tools":[{"name":"list_index_sets","risk":"mutation","requires_confirmation":true}]}}`,
				),
			)
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		cmd := newPermissionCheckCmd(systemtest.BuildDeps(false))
		setFlags(cmd, "bk_biz_id", "100100", "tool", "list_index_sets")
		cmd.SetErr(io.Discard)

		err := cmd.RunE(cmd, nil)
		Expect(err.(*output.CLIError).Code).To(Equal("readonly_contract_changed"))
		Expect(calls).To(Equal([]string{"lookup_tool"}))
	})
})

func toolCommand(name string, dryRun bool) *cobra.Command {
	return newToolCmd(toolsByName[name], systemtest.BuildDeps(dryRun))
}

func dryRunArgs(cmd *cobra.Command) map[string]any {
	stdout, err := systemtest.CaptureCommandStdout(func() error { return cmd.RunE(cmd, nil) })
	Expect(err).NotTo(HaveOccurred())
	var env map[string]any
	Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
	return previewArgs(env)
}

func checkServer(calls *[]string, permission string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := facade(r.URL.Path)
		*calls = append(*calls, name)
		switch name {
		case "lookup_tool":
			_, _ = w.Write(
				[]byte(
					`{"result":true,"data":{"tools":[{"name":"get_index_set_fields","risk":"query","requires_confirmation":false,"permission_state":"requires_resource"}]}}`,
				),
			)
		case "lookup_tool_schema":
			_, _ = w.Write(
				[]byte(
					`{"result":true,"data":{"catalog_version":"test","permission":{"fallback_permission":{"resource_type":"indices","resource_arg":"index_set_id"}}}}`,
				),
			)
		default:
			body, _ := io.ReadAll(r.Body)
			Expect(string(body)).NotTo(ContainSubstring("query_string"))
			_, _ = w.Write([]byte(permission))
		}
	}))
}

func facade(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	return parts[len(parts)-1]
}

func setFlags(cmd *cobra.Command, values ...string) {
	for i := 0; i < len(values); i += 2 {
		Expect(cmd.Flags().Set(values[i], values[i+1])).To(Succeed())
	}
}

// previewArgs reads the business arguments at their documented HTTP location.
func previewArgs(env map[string]any) map[string]any {
	request := env["request"].(map[string]any)
	if body, ok := request["body"].(map[string]any); ok {
		return body
	}
	return request["params"].(map[string]any)
}

func readMonitorRequest(r *http.Request) map[string]any {
	if r.Method != http.MethodGet {
		var body map[string]any
		Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
		return body
	}
	result := map[string]any{}
	for key, values := range r.URL.Query() {
		result[key] = values[0]
		for _, param := range toolsByName[facade(r.URL.Path)].Params {
			if param.Name == key && param.Type != "string" {
				var value any
				Expect(json.Unmarshal([]byte(values[0]), &value)).To(Succeed())
				result[key] = value
			}
		}
	}
	return result
}
