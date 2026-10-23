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
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	"github.com/TencentBlueKing/bk-cli/internal/update"
)

// Synthetic scopes and resources; not a deployment configuration.
const testProfiles = `version: 1
profiles:
  worker: &base
    description: worker logs and metrics
    context: default
    bk_biz_id: "100100"
    log: {index_set_id: 101003}
    metric: {table_id: example.table}
    apm: {app_name: checkout, service_name: api}
  ns-a:
    <<: *base
    log:
      scene: k8s
      where: {cluster_id: test-cluster, namespace: ns-a}
  ns-b:
    <<: *base
    log:
      scene: k8s
      where: {cluster_id: test-cluster, namespace: ns-b}
`

var _ = Describe("SRE profiles", func() {
	var dir, file string
	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", dir)
		GinkgoT().Setenv("BK_CLI_MONITOR_PROFILE_FILE", "")
		file = filepath.Join(dir, "profiles.yaml")
		Expect(os.WriteFile(file, []byte(testProfiles), 0o600)).To(Succeed())
	})

	It("supports YAML anchors, named profiles and exact integer identifiers", func() {
		value, hash, err := loadProfiles(file)
		Expect(err).NotTo(HaveOccurred())
		Expect(hash).To(HaveLen(64))
		Expect(value.Profiles["ns-b"].Metric.TableID).To(Equal("example.table"))
		Expect(value.Profiles["ns-b"].Log.Where["namespace"]).To(Equal("ns-b"))
		raw := strings.Replace(testProfiles, "101003", "9007199254740993", 1)
		Expect(os.WriteFile(file, []byte(raw), 0o600)).To(Succeed())
		value, _, err = loadProfiles(file)
		Expect(err).NotTo(HaveOccurred())
		Expect(value.Profiles["worker"].Log.IndexSetID).To(Equal(int64(9007199254740993)))
	})

	DescribeTable(
		"fails closed on invalid profiles without echoing parser values",
		func(raw string) {
			Expect(os.WriteFile(file, []byte(raw), 0o600)).To(Succeed())
			_, _, err := loadProfiles(file)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring("secret-value"))
		},
		Entry("unknown version", strings.Replace(testProfiles, "version: 1", "version: 2", 1)),
		Entry(
			"token",
			strings.Replace(
				testProfiles,
				"context: default",
				"context: default\n    access_token: secret-value",
				1,
			),
		),
		Entry(
			"URL instead of context",
			strings.Replace(testProfiles, "context: default", "context: https://secret-value", 1),
		),
		Entry(
			"both log modes",
			strings.Replace(
				testProfiles,
				"{index_set_id: 101003}",
				"{index_set_id: 101003, scene: host}",
				1,
			),
		),
		Entry(
			"where without scene",
			strings.Replace(
				testProfiles,
				"{index_set_id: 101003}",
				"{index_set_id: 101003, where: {namespace: ns-a}}",
				1,
			),
		),
		Entry(
			"conditions without index",
			strings.Replace(
				testProfiles,
				"{index_set_id: 101003}",
				"{scene: host, conditions: {key: value}}",
				1,
			),
		),
		Entry(
			"duplicate key",
			strings.Replace(testProfiles, "context: default", "context: default\n    context: default", 1),
		),
		Entry("multiple YAML documents", testProfiles+"\n---\nversion: 1\n"),
		Entry("empty business", strings.Replace(testProfiles, `bk_biz_id: "100100"`, `bk_biz_id: ""`, 1)),
		Entry("unsafe name", strings.Replace(testProfiles, "worker: &base", "'bad;name': &base", 1)),
	)

	It("bounds file sizes and accepts negative business spaces", func() {
		Expect(os.WriteFile(file, []byte(strings.Repeat("x", maxInputBytes+1)), 0o600)).To(Succeed())
		_, _, err := loadProfiles(file)
		Expect(err).To(HaveOccurred())
		Expect(
			os.WriteFile(file, []byte(strings.Replace(testProfiles, `"100100"`, `"-51"`, 1)), 0o600),
		).To(
			Succeed(),
		)
		_, _, err = loadProfiles(file)
		Expect(err).NotTo(HaveOccurred())
	})

	It("lists and shows files with no context or credentials installed", func() {
		cmd := newProfileListCmd()
		setFlags(cmd, "profile_file", file)
		env, err := runProfileCommand(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(env["data"].(map[string]any)["profiles"]).To(HaveLen(3))
		cmd = newProfileShowCmd(systemtest.BuildDeps(false))
		setFlags(cmd, "profile", "worker", "profile_file", file)
		env, err = runProfileCommand(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(env["data"].(map[string]any)["context"]).To(Equal("default"))
		Expect(config.ContextExists("default")).To(BeFalse())
	})

	It("resolves an explicit file before the env file and never selects a default profile", func() {
		GinkgoT().Setenv("BK_CLI_MONITOR_PROFILE_FILE", filepath.Join(dir, "missing.yaml"))
		cmd := newProfileShowCmd(systemtest.BuildDeps(false))
		setFlags(cmd, "profile", "worker", "profile_file", file)
		_, err := runProfileCommand(cmd)
		Expect(err).NotTo(HaveOccurred())
		cmd = newProfileShowCmd(systemtest.BuildDeps(false))
		_, err = runProfileCommand(cmd)
		Expect(err.(*output.CLIError).Code).To(Equal("needs_profile"))
	})

	It("fails on missing profiles and context/business mismatches before making requests", func() {
		deps := systemtest.BuildDeps(false)
		deps.GetContext = func() string { return "other" }
		cmd := newToolCmd(toolsByName["get_index_set_fields"], deps)
		setFlags(cmd, "profile", "worker", "profile_file", file)
		_, err := runProfileCommand(cmd)
		Expect(err.(*output.CLIError).Code).To(Equal("profile_context_mismatch"))
		cmd = toolCommand("get_index_set_fields", false)
		setFlags(cmd, "profile", "worker", "profile_file", file, "bk_biz_id", "200200")
		_, err = runProfileCommand(cmd)
		Expect(err.(*output.CLIError).Code).To(Equal("profile_business_mismatch"))
		cmd = toolCommand("get_index_set_fields", false)
		setFlags(cmd, "profile", "unknown", "profile_file", file)
		_, err = runProfileCommand(cmd)
		Expect(err.(*output.CLIError).Code).To(Equal("profile_not_found"))
	})

	It("fills only accepted tool parameters, preserving flags > body > profile", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		cmd := toolCommand("get_index_set_fields", true)
		setFlags(
			cmd,
			"profile",
			"worker",
			"profile_file",
			file,
			"body",
			`{"index_set_id":111}`,
			"index_set_id",
			"222",
		)
		args := dryRunArgs(cmd)
		Expect(args).To(Equal(map[string]any{"bk_biz_id": "100100", "index_set_id": float64(222)}))
		cmd = toolCommand("list_time_series_metrics", true)
		setFlags(cmd, "profile", "worker", "profile_file", file)
		args = dryRunArgs(cmd)
		Expect(args).To(Equal(map[string]any{"bk_biz_id": "100100", "table_id": "example.table"}))
		cmd = toolCommand("get_index_set_fields", true)
		setFlags(cmd, "profile", "worker", "profile_file", file, "body", `{"bk_biz_id":"200200"}`)
		_, err := runProfileCommand(cmd)
		Expect(err.(*output.CLIError).Code).To(Equal("profile_business_mismatch"))
	})

	It("applies scene namespace profiles deterministically and overrides where per key", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		cmd := toolCommand("search_logs", true)
		setFlags(cmd, "profile", "ns-a", "profile_file", file, "where", "namespace=ns-b", "since", "15m")
		args := dryRunArgs(cmd)
		Expect(args["target_type"]).To(Equal("scene"))
		clauses := args["table_id_conditions"].([]any)[0].([]any)
		Expect(clauses).To(HaveLen(3))
		Expect(clauses[2].(map[string]any)["value"]).To(Equal([]any{"ns-b"}))
	})

	DescribeTable("does not silently switch log modes", func(profile, name, key, value string) {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		cmd := toolCommand(name, true)
		setFlags(cmd, "profile", profile, "profile_file", file)
		if key != "" {
			setFlags(cmd, key, value)
		}
		_, err := runProfileCommand(cmd)
		Expect(err.(*output.CLIError).Code).To(Equal("profile_log_mode_mismatch"))
	},
		Entry("index to scene", "worker", "search_logs", "scene", "host"),
		Entry("scene to index", "ns-a", "search_logs", "index_set_id", "101003"),
		Entry("scene to different scene", "ns-a", "search_logs", "scene", "host"),
		Entry("index-only context tool", "ns-a", "search_index_set_context", "", ""),
	)

	It("keeps response data compatible and makes one direct execute call with provenance", func() {
		calls := []string{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			calls = append(calls, facade(r.URL.Path))
			req := readMonitorRequest(r)
			Expect(
				req,
			).To(
				Equal(map[string]any{"bk_biz_id": "100100", "index_set_id": float64(101003)}),
			)
			_, _ = w.Write(
				[]byte(
					`{"result":true,"data":{"fields":[]}}`,
				),
			)
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		cmd := toolCommand("get_index_set_fields", false)
		setFlags(cmd, "profile", "worker", "profile_file", file)
		env, err := runProfileCommand(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(calls).To(Equal([]string{"get_index_set_fields"}))
		Expect(env["data"]).To(Equal(map[string]any{"fields": []any{}}))
		Expect(env["profile"].(map[string]any)["applied_fields"]).To(Equal([]any{"bk_biz_id", "index_set_id"}))
	})

	It("uses profile resources for permission apply without executing business tools", func() {
		calls := []string{}
		server := checkServer(
			&calls,
			`{"result":true,"data":{"authorized":true,"scopes":[{"tool_name":"get_index_set_fields","state":"granted"}]}}`,
		)
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		cmd := newPermissionApplyCmd(systemtest.BuildDeps(false))
		setFlags(cmd, "profile", "worker", "profile_file", file, "tool", "get_index_set_fields")
		env, err := runProfileCommand(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(env["data"].(map[string]any)["decision"]).To(Equal("ready"))
		Expect(calls).NotTo(ContainElement("execute_tool"))
	})

	It("retains --from anchor mismatch protection with a profile", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		hit := filepath.Join(dir, "hit.json")
		Expect(
			os.WriteFile(
				hit,
				[]byte(
					`{"__index_set_id__":999,"dtEventTimeStamp":"1","serverIp":"192.0.2.10","gseIndex":"1"}`,
				),
				0o600,
			),
		).To(
			Succeed(),
		)
		cmd := toolCommand("search_index_set_context", true)
		setFlags(cmd, "profile", "worker", "profile_file", file, "from", hit)
		_, err := runProfileCommand(cmd)
		Expect(err.(*output.CLIError).Code).To(Equal("anchor_index_mismatch"))
	})

	It("checks configured resources through metadata only and catches an invalid metric table", func() {
		calls := []string{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			req := readMonitorRequest(r)
			name, _ := req["tool_name"].(string)
			op := facade(r.URL.Path)
			if name == "" {
				name = op
				calls = append(calls, op)
			} else {
				calls = append(calls, op+":"+name)
			}
			data := map[string]any{}
			switch op {
			case "lookup_tool":
				data["tools"] = []any{
					map[string]any{"name": name, "risk": "query", "requires_confirmation": false},
				}
			case "lookup_tool_schema":
				data["catalog_version"] = catalogData.Version
				data["permission"] = map[string]any{
					"fallback_permission": map[string]any{"resource_arg": "index_set_id"},
				}
			case "lookup_permissions":
				data["authorized"] = true
				data["scopes"] = []any{map[string]any{"tool_name": name, "state": "granted"}}
			case "get_index_set_fields", "list_time_series_metrics", "list_apm_services":
				Expect(
					name,
				).To(
					BeElementOf(
						"get_index_set_fields",
						"list_time_series_metrics",
						"list_apm_services",
					),
				)
				if name == "list_time_series_metrics" {
					_, _ = w.Write([]byte(`{"result":false,"message":"table does not exist"}`))
					return
				}
				data = map[string]any{}
			}
			Expect(json.NewEncoder(w).Encode(map[string]any{"result": true, "data": data})).To(Succeed())
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		cmd := newProfileCheckCmd(systemtest.BuildDeps(false))
		setFlags(cmd, "profile", "worker", "profile_file", file)
		env, err := runProfileCommand(cmd)
		Expect(err.(*output.CLIError).ExitCode).To(Equal(1))
		encoded, _ := json.Marshal(env)
		Expect(string(encoded)).To(ContainSubstring("resource_not_found"))
		Expect(calls).To(ContainElement("list_time_series_metrics"))
		Expect(calls).NotTo(ContainElement("search_logs"))
	})

	It("reports catalog drift as a warning, not proof that authentication failed", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"result":true,"data":{"catalog_version":"different"}}`))
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		env, err := runProfileCommand(newStatusCmd(systemtest.BuildDeps(false)))
		Expect(err).NotTo(HaveOccurred())
		Expect(env["ok"]).To(BeTrue())
		encoded, _ := json.Marshal(env)
		Expect(string(encoded)).To(ContainSubstring(`"remote":"different","status":"warn"`))
	})

	It("keeps offline/dry-run status network-free and leaves active context untouched", func() {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		before, err := os.ReadFile(filepath.Join(dir, "current"))
		Expect(err).NotTo(HaveOccurred())
		for _, dry := range []bool{false, true} {
			cmd := newStatusCmd(systemtest.BuildDeps(dry))
			setFlags(cmd, "profile", "worker", "profile_file", file, "check_update", "true")
			if !dry {
				setFlags(cmd, "offline", "true")
			}
			env, err := runProfileCommand(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(env["ok"]).To(BeTrue())
		}
		Expect(calls).To(Equal(0))
		after, err := os.ReadFile(filepath.Join(dir, "current"))
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
	})

	It("maps APM service_names only when accepted and does not rewrite PromQL", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		cmd := toolCommand("list_apm_services", true)
		setFlags(cmd, "profile", "worker", "profile_file", file)
		args := dryRunArgs(cmd)
		Expect(args["service_names"]).To(Equal([]any{"api"}))
		Expect(args).NotTo(HaveKey("service_name"))
		cmd = toolCommand("execute_range_query", true)
		setFlags(cmd, "profile", "worker", "profile_file", file, "promql", "up", "since", "15m")
		args = dryRunArgs(cmd)
		Expect(args["promql"]).To(Equal("up"))
		Expect(args).NotTo(HaveKey("table_id"))
	})

	It("keeps structured conditions scoped to log search and allows explicit object replacement", func() {
		raw := strings.Replace(
			testProfiles,
			"log: {index_set_id: 101003}",
			"log: {index_set_id: 101003, conditions: {condition: and, field_list: []}}",
			1,
		)
		Expect(os.WriteFile(file, []byte(raw), 0o600)).To(Succeed())
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		cmd := toolCommand("search_logs", true)
		setFlags(cmd, "profile", "worker", "profile_file", file, "since", "15m")
		args := dryRunArgs(cmd)
		Expect(args["conditions"]).To(HaveKeyWithValue("condition", "and"))
		cmd = toolCommand("search_logs", true)
		setFlags(
			cmd,
			"profile",
			"worker",
			"profile_file",
			file,
			"since",
			"15m",
			"conditions",
			`{"condition":"or","field_list":[]}`,
		)
		Expect(dryRunArgs(cmd)["conditions"]).To(HaveKeyWithValue("condition", "or"))
		cmd = toolCommand("list_alerts", true)
		setFlags(cmd, "profile", "worker", "profile_file", file, "since", "15m")
		Expect(dryRunArgs(cmd)).NotTo(HaveKey("conditions"))
	})

	It("reports comparable releases, local builds and unavailable release sources accurately", func() {
		Expect(
			releaseCheck("0.1.0", &update.ReleaseInfo{Version: "0.2.0"}, nil),
		).To(
			HaveKeyWithValue("newer_release", true),
		)
		Expect(
			releaseCheck("0.2.0", &update.ReleaseInfo{Version: "0.2.0"}, nil),
		).To(
			HaveKeyWithValue("status", "pass"),
		)
		preview := releaseCheck("observe-preview", &update.ReleaseInfo{Version: "0.2.0"}, nil)
		Expect(preview).To(HaveKeyWithValue("decision", "version_not_comparable"))
		Expect(preview).NotTo(HaveKey("newer_release"))
		failed := releaseCheck("0.2.0", nil, errors.New("secret upstream error"))
		Expect(failed).To(HaveKeyWithValue("decision", "update_check_unavailable"))
		Expect(failed).NotTo(HaveKey("message"))
	})

	It("reports an actual authenticated API failure rather than credential-file success", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(
				[]byte(`{"result":false,"code":403,"message":"access_token expired secret-value"}`),
			)
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		cmd := newStatusCmd(systemtest.BuildDeps(false))
		env, err := runProfileCommand(cmd)
		Expect(err.(*output.CLIError).ExitCode).To(Equal(1))
		Expect(env["ok"]).To(BeFalse())
		encoded, _ := json.Marshal(env)
		Expect(string(encoded)).To(ContainSubstring("auth_failed"))
		Expect(string(encoded)).NotTo(ContainSubstring("secret-value"))
	})
})

func runProfileCommand(cmd *cobra.Command) (map[string]any, error) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetContext(context.Background())
	err := cmd.RunE(cmd, nil)
	env := map[string]any{}
	if stdout.Len() > 0 {
		Expect(json.Unmarshal(stdout.Bytes(), &env)).To(Succeed())
	}
	return env, err
}
