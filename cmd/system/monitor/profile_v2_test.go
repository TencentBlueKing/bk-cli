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

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

// Synthetic scopes and resources; not a deployment configuration.
const testProfilesV2 = `version: 2
sources:
  logs: {context: default, bk_biz_id: "200200"}
  traces: {context: cloud, bk_biz_id: "100100"}
profiles:
  monitor-prod:
    environment: production
    components:
      backend:
        bindings:
          log:
            module: log
            source: logs
            args: {index_set_id: 101001}
          trace: &trace
            module: apm
            source: traces
            args: {app_name: app-production}
            tools:
              search_spans:
                filters: [{key: service.name, operator: equal, value: [api]}]
          traffic:
            module: metric
            source: traces
            args: {table_id: metrics.api}
            tools:
              execute_range_query: {promql: "sum(request_qps)"}
          latency:
            module: metric
            source: traces
            args: {table_id: metrics.api}
            tools:
              execute_range_query: {promql: "max(request_latency)"}
          policy:
            module: alarm
            source: logs
            tools:
              get_alarm_strategy:
                conditions: [{key: strategy_id, value: ["123"]}]
      worker:
        bindings:
          log:
            module: log
            source: logs
            args: {index_set_id: 101002}
          trace: *trace
`

func v2Deps(dry bool) systemcmd.BuildDeps {
	deps := systemtest.BuildDeps(dry)
	deps.GetContext = func() string { return "" }
	return deps
}

var _ = Describe("heterogeneous profiles v2", func() {
	var dir, file string
	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", dir)
		GinkgoT().Setenv("BK_CLI_MONITOR_PROFILE_FILE", "")
		file = filepath.Join(dir, "v2.yaml")
		Expect(os.WriteFile(file, []byte(testProfilesV2), 0o600)).To(Succeed())
	})

	It("shows environment/components/bindings without credentials or a selected route", func() {
		cmd := newProfileListCmd()
		setFlags(cmd, "profile_file", file)
		env, err := runProfileCommand(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(
			env["data"].(map[string]any)["profiles"].([]any)[0],
		).To(
			HaveKeyWithValue("components", []any{"backend", "worker"}),
		)
		show := newProfileShowCmd(v2Deps(false))
		setFlags(show, "profile", "monitor-prod", "profile_file", file)
		env, err = runProfileCommand(show)
		Expect(err).NotTo(HaveOccurred())
		Expect(env["data"].(map[string]any)["components"]).To(HaveLen(2))
		Expect(env["profile"]).NotTo(HaveKey("context"))
		Expect(config.ContextExists("default")).To(BeFalse())
	})

	DescribeTable(
		"rejects invalid references and ambiguous native defaults",
		func(old, new string) {
			Expect(
				os.WriteFile(file, []byte(strings.Replace(testProfilesV2, old, new, 1)), 0o600),
			).To(
				Succeed(),
			)
			_, _, err := loadProfiles(file)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring("SECRET"))
		},
		Entry("unknown source", "source: logs", "source: nowhere"),
		Entry("cross-module preset", "search_spans:", "list_alerts:"),
		Entry("mutation preset", "search_spans:", "create_dashboard:"),
		Entry(
			"credential in source",
			`context: default, bk_biz_id: "200200"`,
			`context: default, bk_biz_id: "200200", access_token: SECRET`,
		),
		Entry("ambiguous id", `args: {index_set_id: 101001}`, `args: {id: 101001}`),
		Entry("wrong native type", `args: {index_set_id: 101001}`, `args: {index_set_id: "101001"}`),
		Entry(
			"unknown tool arg",
			`execute_range_query: {promql: "sum(request_qps)"}`,
			`execute_range_query: {shell: SECRET}`,
		),
		Entry(
			"stale time",
			`execute_range_query: {promql: "sum(request_qps)"}`,
			`execute_range_query: {start_time: "1"}`,
		),
	)

	DescribeTable("never guesses a component or a named query", func(tool, component, binding, code string) {
		cmd := newToolCmd(toolsByName[tool], v2Deps(true))
		setFlags(cmd, "profile", "monitor-prod", "profile_file", file)
		if component != "" {
			setFlags(cmd, "component", component)
		}
		if binding != "" {
			setFlags(cmd, "binding", binding)
		}
		_, err := runProfileCommand(cmd)
		Expect(err.(*output.CLIError).Code).To(Equal(code))
	},
		Entry("component required", "search_logs", "", "", "needs_component"),
		Entry("golden metric required", "execute_range_query", "backend", "", "needs_binding"),
		Entry("not mounted on worker", "execute_range_query", "worker", "", "profile_binding_missing"),
		Entry("wrong capability", "search_logs", "backend", "trace", "profile_binding_mismatch"),
		Entry("missing component", "search_logs", "unknown", "", "component_not_found"),
	)

	It("selects per-command context and business without touching the active context", func() {
		requests := make(chan map[string]any, 4)
		handler := func(label string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				req := readMonitorRequest(r)
				requests <- map[string]any{"server": label, "body": req}
				Expect(
					json.NewEncoder(
						w,
					).Encode(
						map[string]any{
							"result": true,
							"data":   map[string]any{},
						},
					),
				).To(
					Succeed(),
				)
			}
		}
		logs := httptest.NewServer(handler("logs"))
		cloud := httptest.NewServer(handler("traces"))
		DeferCleanup(logs.Close)
		DeferCleanup(cloud.Close)
		Expect(systemtest.SetupTestContext(logs.URL)).To(Succeed())
		Expect(
			config.CreateContext("cloud", &config.Config{BkAPIURLTmpl: cloud.URL + "/{gateway_name}"}),
		).To(
			Succeed(),
		)
		cred, err := os.ReadFile(config.CredentialsPath("default"))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(config.CredentialsPath("cloud"), cred, 0o600)).To(Succeed())
		for _, tc := range []struct {
			tool, component, server, biz string
			index                        float64
		}{{"get_index_set_fields", "backend", "logs", "200200", 101001}, {"get_index_set_fields", "worker", "logs", "200200", 101002}, {"search_spans", "backend", "traces", "100100", 0}} {
			cmd := newToolCmd(toolsByName[tc.tool], v2Deps(false))
			setFlags(cmd, "profile", "monitor-prod", "profile_file", file, "component", tc.component)
			if tc.tool == "search_spans" {
				setFlags(cmd, "since", "15m", "limit", "1")
			}
			env, err := runProfileCommand(cmd)
			Expect(err).NotTo(HaveOccurred())
			r := <-requests
			Expect(r["server"]).To(Equal(tc.server))
			args := r["body"].(map[string]any)
			Expect(args["bk_biz_id"]).To(Equal(tc.biz))
			if tc.index > 0 {
				Expect(args["index_set_id"]).To(Equal(tc.index))
			} else {
				Expect(args["filters"]).To(HaveLen(1))
			}
			Expect(env["profile"]).To(HaveKeyWithValue("component", tc.component))
		}
		active, err := config.ActiveContextName()
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(Equal("default"))
	})

	It("uses named query presets but lets explicit arguments override them", func() {
		Expect(systemtest.SetupTestContext("https://api.example.com")).To(Succeed())
		Expect(
			config.CreateContext(
				"cloud",
				&config.Config{BkAPIURLTmpl: "https://cloud.example.com/{gateway_name}"},
			),
		).To(
			Succeed(),
		)
		cred, err := os.ReadFile(config.CredentialsPath("default"))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(config.CredentialsPath("cloud"), cred, 0o600)).To(Succeed())
		cmd := newToolCmd(toolsByName["execute_range_query"], v2Deps(true))
		setFlags(
			cmd,
			"profile",
			"monitor-prod",
			"profile_file",
			file,
			"component",
			"backend",
			"binding",
			"traffic",
			"since",
			"15m",
		)
		args := dryRunArgs(cmd)
		Expect(args["promql"]).To(Equal("sum(request_qps)"))
		Expect(args).NotTo(HaveKey("table_id"))
		cmd = newToolCmd(toolsByName["execute_range_query"], v2Deps(true))
		setFlags(
			cmd,
			"profile",
			"monitor-prod",
			"profile_file",
			file,
			"component",
			"backend",
			"binding",
			"traffic",
			"since",
			"15m",
			"promql",
			"up",
		)
		Expect(dryRunArgs(cmd)["promql"]).To(Equal("up"))
	})

	It("uses exact policy parameters without applying an ID to unrelated tools", func() {
		Expect(systemtest.SetupTestContext("https://api.example.com")).To(Succeed())
		cmd := newToolCmd(toolsByName["get_alarm_strategy"], v2Deps(true))
		setFlags(cmd, "profile", "monitor-prod", "profile_file", file, "component", "backend")
		args := dryRunArgs(cmd)
		Expect(args["conditions"]).To(HaveLen(1))
		Expect(args["bk_biz_id"]).To(Equal("200200"))
		cmd = newToolCmd(toolsByName["search_alarm_strategies"], v2Deps(true))
		setFlags(cmd, "profile", "monitor-prod", "profile_file", file, "component", "backend")
		Expect(dryRunArgs(cmd)).NotTo(HaveKey("conditions"))
	})

	It("previews all declared routes without requiring contexts or touching the network", func() {
		cmd := newProfileCheckCmd(v2Deps(true))
		setFlags(cmd, "profile", "monitor-prod", "profile_file", file)
		env, err := runProfileCommand(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(env["dry_run"]).To(BeTrue())
		Expect(env["data"].(map[string]any)["plan"]).To(HaveLen(7))
		Expect(config.ContextExists("default")).To(BeFalse())
	})

	It("blocks explicit environment/business conflicts and carries component/binding into hints", func() {
		deps := v2Deps(true)
		deps.GetContext = func() string { return "cloud" }
		cmd := newToolCmd(toolsByName["get_index_set_fields"], deps)
		setFlags(cmd, "profile", "monitor-prod", "profile_file", file, "component", "backend")
		_, err := runProfileCommand(cmd)
		Expect(err.(*output.CLIError).Code).To(Equal("profile_context_mismatch"))
		cmd = newToolCmd(toolsByName["get_index_set_fields"], v2Deps(true))
		setFlags(
			cmd,
			"profile",
			"monitor-prod",
			"profile_file",
			file,
			"component",
			"backend",
			"bk_biz_id",
			"100100",
		)
		_, err = runProfileCommand(cmd)
		Expect(err.(*output.CLIError).Code).To(Equal("profile_business_mismatch"))
		p := &selectedProfile{Version: 2, Name: "monitor-prod", Component: "worker", Binding: "log", File: file}
		Expect(profileSuffix(p)).To(ContainSubstring("--component worker --binding log"))
	})

	It("supports a source-only binding for every module without pretending all have fixed resource IDs", func() {
		for _, module := range modules {
			b := profileBinding{Module: module.Name, Source: "logs"}
			Expect(validateBinding(&b)).To(Succeed(), module.Name)
			for _, name := range v2ProbeTools(b) {
				Expect(toolsByName[name].Module).To(Equal(module.Name))
			}
		}
	})

	It("reports missing sources as failed rather than falling back to the active context", func() {
		cmd := newStatusCmd(v2Deps(false))
		setFlags(cmd, "profile", "monitor-prod", "profile_file", file, "offline", "true")
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		env, err := runProfileCommand(cmd)
		Expect(err.(*output.CLIError).Code).To(Equal("monitor_check_failed"))
		Expect(env["ok"]).To(BeFalse())
		Expect(env["data"].(map[string]any)["sources"]).To(HaveLen(2))
	})
})
