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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
	"github.com/TencentBlueKing/bk-cli/internal/config"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
)

const testProfiles = `{"default_env":"dev","environments":{
 "dev":{"space_uid":"trace-space","trace_table_id":"traces","log_space_uid":"log-space",
 "log_table_id":"logs","log_tables":{"stdout":"logs"},"metrics_space_uid":"metric-space","metrics_namespace":"dev-ns"},
 "prod":{"space_uid":"trace-space","trace_table_id":"traces","log_space_uid":"log-space",
 "log_table_id":"logs","log_tables":{"stdout":"logs"},"metrics_space_uid":"metric-space","metrics_namespace":"prod-ns"}}}`

var _ = Describe("monitor commands", func() {
	var path string
	BeforeEach(func() {
		dir, err := os.MkdirTemp("", "bk-cli-monitor-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, dir)
		for _, name := range []string{"BK_CLI_CONFIG_DIR", "BK_CLI_MONITOR_CONFIG", "BK_CLI_MONITOR_ENV", "BK_CLI_MONITOR_SPACE_UID"} {
			old, exists := os.LookupEnv(name)
			DeferCleanup(func() {
				if exists {
					Expect(os.Setenv(name, old)).To(Succeed())
				} else {
					Expect(os.Unsetenv(name)).To(Succeed())
				}
			})
			Expect(os.Unsetenv(name)).To(Succeed())
		}
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", dir)).To(Succeed())
		path = filepath.Join(dir, "monitor.json")
		Expect(os.WriteFile(path, []byte(testProfiles), 0o600)).To(Succeed())
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
	})

	run := func(cmd *cobra.Command, args ...string) (string, error) {
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(io.Discard)
		cmd.SilenceErrors, cmd.SilenceUsage = true, true
		cmd.SetArgs(append([]string{"--config", path}, args...))
		err := cmd.Execute()
		return out.String(), err
	}

	It("loads monitor profiles from the bk-cli config directory by default", func() {
		Expect(configPath()).To(Equal(path))
		cmd := newEnvsCmd(systemtest.BuildDeps(false))
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{})
		Expect(cmd.Execute()).To(Succeed())
		Expect(out.String()).To(ContainSubstring(`"default_env": "dev"`))
		Expect(out.String()).To(ContainSubstring(`"prod"`))
	})

	It("prefers explicit config over the monitor config environment variable", func() {
		other := filepath.Join(filepath.Dir(path), "other.json")
		Expect(os.WriteFile(other, []byte(`{"default_env":"other"}`), 0o600)).To(Succeed())
		Expect(os.Setenv("BK_CLI_MONITOR_CONFIG", other)).To(Succeed())
		Expect(configPath()).To(Equal(other))
		out, err := run(newEnvsCmd(systemtest.BuildDeps(false)))
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`"default_env": "dev"`))
	})

	preview := func(kind string, args ...string) map[string]any {
		out, err := run(newQueryCmd(kind, systemtest.BuildDeps(true)), args...)
		Expect(err).NotTo(HaveOccurred())
		var env map[string]any
		Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())
		Expect(env["dry_run"]).To(BeTrue())
		return env["request"].(map[string]any)
	}

	It("routes logs and named sources into the configured space with AND filters", func() {
		request := preview(
			"logs",
			"--log-source",
			"stdout",
			"--where",
			"service=api",
			"--query",
			"ERROR",
			"--from",
			"20",
		)
		Expect(request["url"]).To(Equal("https://bkapi.example.com/api/bk-unify-query/prod/query/ts/raw"))
		Expect(request["headers"].(map[string]any)["X-Bk-Scope-Space-Uid"]).To(Equal("log-space"))
		q := request["body"].(map[string]any)["query_list"].([]any)[0].(map[string]any)
		Expect(q["table_id"]).To(Equal("logs"))
		Expect(q["query_string"]).To(Equal("ERROR"))
		Expect(q["conditions"]).NotTo(BeNil())
	})

	It("uses the trace space and exact trace ID filter", func() {
		request := preview("trace", "--id", "trace-abc", "--where", "service=api")
		Expect(request["headers"].(map[string]any)["X-Bk-Scope-Space-Uid"]).To(Equal("trace-space"))
		q := request["body"].(map[string]any)["query_list"].([]any)[0].(map[string]any)
		fields := q["conditions"].(map[string]any)["field_list"].([]any)
		Expect(fields[1].(map[string]any)["value"]).To(Equal([]any{"trace-abc"}))
	})

	It("scopes metrics and preserves body overrides and large integer inputs", func() {
		raw := `{"query_list":[{"field_name":"custom","unknown":9007199254740993}],"start_time":"1700000000","end_time":"1700003600"}`
		out, err := run(newQueryCmd("metrics", systemtest.BuildDeps(true)), "--body", raw, "--field", "ignored")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("9007199254740993"))
		Expect(out).To(ContainSubstring("dev-ns"))
		Expect(out).To(ContainSubstring("custom"))
		Expect(out).NotTo(ContainSubstring("ignored"))
	})

	It("accepts complete stdin input and moves space_uid to the header", func() {
		cmd := newQueryCmd("logs", systemtest.BuildDeps(true))
		cmd.SetIn(
			strings.NewReader(
				`{"query_list":[{"table_id":"other"}],"start_time":"1","end_time":"2","space_uid":"log-space"}`,
			),
		)
		out, err := run(cmd, "--input", "-")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("other"))
		Expect(out).NotTo(ContainSubstring(`"space_uid"`))
	})

	DescribeTable(
		"rejects malformed or conflicting input before a request",
		func(kind string, args []string) {
			out, err := run(newQueryCmd(kind, systemtest.BuildDeps(true)), args...)
			Expect(err).To(HaveOccurred())
			Expect(out).To(BeEmpty())
		},
		Entry("missing trace ID", "trace", []string{}),
		Entry("negative offset", "logs", []string{"--from", "-1"}),
		Entry("nonpositive limit", "logs", []string{"--limit", "0"}),
		Entry("log source and table", "logs", []string{"--log-source", "stdout", "--table", "other"}),
		Entry("unknown log source", "logs", []string{"--log-source", "absent"}),
		Entry("unknown env", "logs", []string{"--env", "absent"}),
		Entry("two time inputs", "logs", []string{"--start", "1700000000", "--since", "1h"}),
		Entry("input and body", "logs", []string{"--input", "-", "--body", `{}`}),
		Entry("input and filters", "logs", []string{"--input", "-", "--where", "x=y"}),
		Entry("two PromQL aliases", "query", []string{"--promql", "up", "--query", "up"}),
		Entry("invalid step", "query", []string{"--promql", "up", "--step", "0s"}),
		Entry("tenant bypass", "logs", []string{"--header", "X-Bk-Scope-Skip-Space:true"}),
		Entry("space bypass", "logs", []string{"--header", "X-Bk-Scope-Space-Uid:other"}),
		Entry("non-object JSON", "logs", []string{"--body", `[]`}),
		Entry(
			"invalid query list",
			"metrics",
			[]string{"--body", `{"query_list":[1],"start_time":"1","end_time":"2"}`},
		),
	)

	DescribeTable(
		"rejects unsafe metric namespace conditions",
		func(conditions string) {
			raw := `{"query_list":[{"field_name":"metric","conditions":` + conditions + `}],"start_time":"1","end_time":"2"}`
			out, err := run(newQueryCmd("metrics", systemtest.BuildDeps(true)), "--body", raw)
			Expect(err).To(HaveOccurred())
			Expect(out).To(BeEmpty())
		},
		Entry("conflict", `{"field_list":[{"field_name":"namespace","op":"eq","value":["other"]}]}`),
		Entry(
			"OR",
			`{"field_list":[{"field_name":"x","op":"eq","value":["a"]},{"field_name":"y","op":"eq","value":["b"]}],"condition_list":["or"]}`,
		),
		Entry("malformed", `{"field_list":"bad"}`),
	)

	It("expands only explicit namespace tokens and preserves raw PromQL", func() {
		request := preview("query", "--env", "prod", "--promql", `sum(up{namespace={{namespace}}})`)
		Expect(request["body"].(map[string]any)["promql"]).To(Equal(`sum(up{namespace="prod-ns"})`))
		request = preview("query", "--promql", `up{namespace="outside"}`, "--instant")
		Expect(request["body"].(map[string]any)["promql"]).To(Equal(`up{namespace="outside"}`))
	})

	It("preserves namespace quotes and rejects quoted placeholders", func() {
		expression, err := expandPromQL(`up{namespace={{namespace}}}`, "quote\"ns")
		Expect(err).NotTo(HaveOccurred())
		Expect(expression).To(Equal(`up{namespace="quote\"ns"}`))
		_, err = expandPromQL(`up{namespace="{{namespace}}"}`, "dev")
		Expect(err).To(HaveOccurred())
	})

	It("honors explicit env over BK_CLI_MONITOR_ENV and root gateway paths", func() {
		Expect(os.Setenv("BK_CLI_MONITOR_ENV", "prod")).To(Succeed())
		request := preview(
			"query",
			"--env",
			"dev",
			"--promql",
			`up{namespace={{namespace}}}`,
			"--stage",
			"testing",
		)
		Expect(request["body"].(map[string]any)["promql"]).To(Equal(`up{namespace="dev-ns"}`))
		Expect(request["url"]).To(ContainSubstring("/testing/query/promql"))
	})

	It("validates a configured gateway URL against context and supports proxy routing", func() {
		cfg := `{"space_uid":"s","log_table_id":"logs","apigw":{"base_url":"https://bkapi.example.com/api/bk-unify-query/prod","proxy_path":"/proxy"}}`
		Expect(os.WriteFile(path, []byte(cfg), 0o600)).To(Succeed())
		request := preview("logs")
		Expect(request["url"]).To(ContainSubstring("/prod/proxy"))
		Expect(request["body"].(map[string]any)["path"]).To(Equal("/query/ts/raw"))
		cfg = strings.ReplaceAll(cfg, "bkapi.example.com", "other.example.com")
		Expect(os.WriteFile(path, []byte(cfg), 0o600)).To(Succeed())
		_, err := run(newQueryCmd("logs", systemtest.BuildDeps(true)))
		Expect(err).To(HaveOccurred())
	})

	DescribeTable("checks upstream and UQ failures", func(status int, raw string, partial bool) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, raw)
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		out, err := run(newQueryCmd("logs", systemtest.BuildDeps(false)))
		Expect(err).To(HaveOccurred())
		if partial {
			Expect(out).To(ContainSubstring(`"list"`))
		} else {
			Expect(out).To(BeEmpty())
		}
	},
		Entry("HTTP failure", 403, `{"error":"denied"}`, false),
		Entry("gateway denial", 200, `{"result":false,"message":"denied"}`, false),
		Entry("missing gateway data", 200, `{"result":true}`, false),
		Entry("inner denial", 200, `{"result":true,"data":{"error":"denied"}}`, false),
		Entry("partial", 200, `{"result":true,"data":{"list":[],"is_partial":true}}`, true),
		Entry("UQ status", 200, `{"list":[],"status":{"code":"timeout"}}`, true),
		Entry("invalid object", 200, `[]`, false),
	)

	It("unwraps gateway results and redacts echoed credentials", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			Expect(r.Header.Get("X-Bk-Scope-Space-Uid")).To(Equal("log-space"))
			Expect(r.Header.Get("X-Bkapi-Authorization")).To(ContainSubstring("token-123"))
			_, _ = io.WriteString(
				w,
				`{"result":true,"data":{"list":[{"message":"token-123"}],"trace_id":"query-trace"}}`,
			)
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		out, err := run(newQueryCmd("logs", systemtest.BuildDeps(false)))
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("query-trace"))
		Expect(out).To(ContainSubstring("[REDACTED]"))
		Expect(out).NotTo(ContainSubstring("token-123"))
	})

	It("preserves response integers through gateway unwrapping and redaction", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(
				w,
				`{"result":true,"data":{"list":[{"id":9007199254740993,"nested":[18446744073709551615,-9223372036854775809],"message":"token-123"}]}}`,
			)
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		out, err := run(newQueryCmd("logs", systemtest.BuildDeps(false)))
		Expect(err).NotTo(HaveOccurred())
		var env map[string]any
		decoder := json.NewDecoder(strings.NewReader(out))
		decoder.UseNumber()
		Expect(decoder.Decode(&env)).To(Succeed())
		row := env["data"].(map[string]any)["list"].([]any)[0].(map[string]any)
		Expect(row["id"]).To(Equal(json.Number("9007199254740993")))
		Expect(
			row["nested"],
		).To(
			Equal([]any{json.Number("18446744073709551615"), json.Number("-9223372036854775809")}),
		)
		Expect(row["message"]).To(Equal("[REDACTED]"))
	})

	It("redacts overridden auth in response values, keys and gateway headers", func() {
		const auth = `{"bk_app_code":"synthetic-app","bk_app_secret":"override-secret"}`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			Expect(r.Header.Get("X-Bkapi-Authorization")).To(Equal(auth))
			w.Header().Set("X-Bkapi-Authorization", auth)
			_, _ = io.WriteString(w, `{"list":[{"override-secret":"override-secret"}]}`)
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		out, err := run(
			newQueryCmd("logs", systemtest.BuildDeps(false)),
			"--header",
			"X-Bkapi-Authorization:"+auth,
		)
		Expect(err).NotTo(HaveOccurred())
		var env map[string]any
		Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
		headers := env["headers"].(map[string]any)
		Expect(headers["X-Bkapi-Authorization"]).To(Equal("[REDACTED]"))
		list := env["data"].(map[string]any)["list"].([]any)
		Expect(list[0].(map[string]any)["[REDACTED]"]).To(Equal("[REDACTED]"))
		Expect(out).NotTo(ContainSubstring("override-secret"))
	})

	It("checks every environment despite defaults and continues after failures", func() {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			calls++
			var body map[string]any
			Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
			raw := `{"list":[],"trace_id":"query-trace"}`
			if _, ok := body["promql"]; ok || strings.HasSuffix(r.URL.Path, "/query/ts") {
				raw = `{"series":[{"columns":["_value"],"values":[[1]]}],"trace_id":"query-trace"}`
			}
			if calls == 2 {
				raw = `{"result":false}`
			}
			_, _ = fmt.Fprintf(w, "%s", raw)
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		Expect(os.Setenv("BK_CLI_MONITOR_ENV", "dev")).To(Succeed())
		out, err := run(newStatusCmd(systemtest.BuildDeps(false)))
		Expect(err).To(HaveOccurred())
		Expect(calls).To(Equal(10))
		Expect(out).To(ContainSubstring(`"environment": "prod"`))
		Expect(out).To(ContainSubstring(`"failed": 1`))
		Expect(out).To(ContainSubstring(`"empty": 3`))
		Expect(out).NotTo(ContainSubstring(`"values"`))
	})

	It("does not contact upstream for local status or dry-run", func() {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { calls++ }))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		out, err := run(newStatusCmd(systemtest.BuildDeps(false)), "--local")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`"verified": false`))
		out, err = run(newStatusCmd(systemtest.BuildDeps(true)))
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`"dry_run": true`))
		Expect(calls).To(BeZero())
	})

	It("lists environments without credentials and honors --env for status", func() {
		out, err := run(newEnvsCmd(systemtest.BuildDeps(false)))
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`"prod"`))
		out, err = run(newStatusCmd(systemtest.BuildDeps(true)), "--env", "prod")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`"environment": "prod"`))
		Expect(out).NotTo(ContainSubstring(`"environment": "dev"`))
	})
	DescribeTable("preserves ordinary header values and gateway control keys", func(header string) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			name, value, _ := strings.Cut(header, ":")
			Expect(r.Header.Get(name)).To(Equal(value))
			_, _ = io.WriteString(
				w,
				`{"result":true,"data":{"list":[{"message":"dev: synthetic log","namespace":"dev-ns"}],"trace_id":"data"}}`,
			)
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		out, err := run(newQueryCmd("logs", systemtest.BuildDeps(false)), "--header", header)
		Expect(err).NotTo(HaveOccurred())
		var env map[string]any
		Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
		data := env["data"].(map[string]any)
		Expect(data["trace_id"]).To(Equal("data"))
		Expect(
			data["list"].([]any)[0],
		).To(
			Equal(map[string]any{"message": "dev: synthetic log", "namespace": "dev-ns"}),
		)
	}, Entry("tenant", "X-Bk-Tenant-Id:dev"), Entry("request id", "X-Request-Id:data"))

	DescribeTable(
		"interprets control fields before credential redaction",
		func(secret, raw string, partial bool) {
			auth := fmt.Sprintf(`{"bk_app_code":"synthetic-app","bk_app_secret":%q}`, secret)
			server := httptest.NewServer(
				http.HandlerFunc(
					func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, raw) },
				),
			)
			DeferCleanup(server.Close)
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			out, err := run(
				newQueryCmd("logs", systemtest.BuildDeps(false)),
				"--header",
				"X-Bkapi-Authorization:"+auth,
			)
			if partial {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("partial query result"))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(out).To(ContainSubstring("[REDACTED]"))
			Expect(out).To(ContainSubstring(`"ok": true`))
		},
		Entry("gateway data key", "data", `{"result":true,"data":{"list":[{"message":"data"}]}}`, false),
		Entry(
			"partial status code key",
			"code",
			`{"result":true,"data":{"list":[],"status":{"code":"partial"}}}`,
			true,
		),
	)

	It("redacts only known credential fields and bearer tokens", func() {
		runtime := &syslib.Runtime{}
		secrets := responseSecrets(
			runtime,
			[]string{
				`X-Bkapi-Authorization:{"bk_app_code":"app","bk_username":"user","bk_app_secret":"secret","bk_ticket":"ticket","bk_token":1,"custom":"public","metadata":1}`,
				"Authorization:Bearer bearer-token",
				"Proxy-Authorization:Basic proxy-token",
				"X-Request-Id:request",
				"X-Bk-Tenant-Id:dev",
			},
		)
		text := redactString("secret ticket bearer-token proxy-token app user public request dev 1", secrets)
		Expect(text).To(Equal("[REDACTED] [REDACTED] [REDACTED] [REDACTED] app user public request dev 1"))
		Expect(
			responseSecrets(runtime, []string{"X-Bkapi-Authorization:opaque-auth"}),
		).To(
			ContainElement("opaque-auth"),
		)
	})

	It("transmits explicit Unix and RFC3339 times and sort fields", func() {
		request := preview(
			"logs",
			"--start",
			"2023-11-14T22:13:20Z",
			"--end",
			"1700003600",
			"--order-by",
			" -time, ,id ",
		)
		body := request["body"].(map[string]any)
		Expect(body["start_time"]).To(Equal("1700000000"))
		Expect(body["end_time"]).To(Equal("1700003600"))
		Expect(body["order_by"]).To(Equal([]any{"-time", "id"}))
	})

	It("reads query files without rounding input and reports file read failures", func() {
		input := filepath.Join(filepath.Dir(path), "query.json")
		Expect(
			os.WriteFile(
				input,
				[]byte(
					`{"query_list":[{"table_id":"file-logs","id":9007199254740993}],"start_time":"1","end_time":"2"}`,
				),
				0o600,
			),
		).To(
			Succeed(),
		)
		out, err := run(newQueryCmd("logs", systemtest.BuildDeps(true)), "--input", input)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("file-logs"))
		Expect(out).To(ContainSubstring("9007199254740993"))
		_, err = run(newQueryCmd("logs", systemtest.BuildDeps(true)), "--input", input+"-missing")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no such file"))
	})

	It("permits a missing optional default config but rejects explicit, invalid and unreadable configs", func() {
		missing := filepath.Join(filepath.Dir(path), "missing.json")
		root, err := readProfiles(missing, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(root.Environments).To(BeNil())
		_, err = readProfiles(missing, true)
		Expect(err).To(HaveOccurred())
		Expect(os.Setenv("BK_CLI_MONITOR_CONFIG", missing)).To(Succeed())
		_, err = readProfiles(missing, false)
		Expect(err).To(HaveOccurred())
		Expect(os.WriteFile(path, []byte(`{broken`), 0o600)).To(Succeed())
		for _, cmd := range []*cobra.Command{newEnvsCmd(systemtest.BuildDeps(false)), newStatusCmd(systemtest.BuildDeps(false)), newQueryCmd("logs", systemtest.BuildDeps(true))} {
			_, err = run(cmd)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid monitor configuration JSON"))
		}
		_, err = readProfiles(filepath.Dir(path), false)
		Expect(err).To(HaveOccurred())
	})

	It("honors matching space headers and rejects malformed or conflicting scopes", func() {
		headers, err := scopedHeaders(map[string]any{"space_uid": "s"}, "s", []string{"x-bk-scope-space-uid:s"})
		Expect(err).NotTo(HaveOccurred())
		Expect(headers).To(Equal([]string{"x-bk-scope-space-uid:s"}))
		for _, test := range []struct {
			space   string
			body    map[string]any
			headers []string
			message string
		}{
			{"", map[string]any{}, nil, "configure a query space"},
			{"s", map[string]any{}, []string{"invalid"}, "header"},
			{"s", map[string]any{"space_uid": json.Number("1")}, nil, "input space_uid conflicts"},
			{"s", map[string]any{}, []string{"X-Bk-Scope-Space-Uid:other"}, "space header conflicts"},
		} {
			_, err := scopedHeaders(test.body, test.space, test.headers)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(test.message))
		}
	})

	It("routes custom paths, stage overrides and proxy bodies for metrics and PromQL", func() {
		configured := `{"space_uid":"s","log_table_id":"l","apigw":{"gateway_name":"custom","stage":"stag","query_raw_path":"/raw","query_ts_path":"/metrics","query_promql_path":"/promql"}}`
		Expect(os.WriteFile(path, []byte(configured), 0o600)).To(Succeed())
		Expect(preview("logs")["url"]).To(Equal("https://bkapi.example.com/api/custom/stag/raw"))
		Expect(
			preview("metrics", "--field", "cpu")["url"],
		).To(
			Equal("https://bkapi.example.com/api/custom/stag/metrics"),
		)
		Expect(
			preview("query", "--promql", "vector(1)", "--stage", "test")["url"],
		).To(
			Equal("https://bkapi.example.com/api/custom/test/promql"),
		)
		Expect(
			os.WriteFile(path, []byte(`{"space_uid":"s","apigw":{"proxy_path":"/proxy"}}`), 0o600),
		).To(
			Succeed(),
		)
		for _, test := range []struct {
			kind, path string
			args       []string
		}{{"metrics", "/query/ts", []string{"--field", "cpu"}}, {"query", "/query/ts/promql", []string{"--promql", "vector(1)"}}} {
			request := preview(test.kind, test.args...)
			Expect(request["url"]).To(HaveSuffix("/proxy"))
			body := request["body"].(map[string]any)
			Expect(body["path"]).To(Equal(test.path))
			Expect(body["data"]).To(BeAssignableToTypeOf(map[string]any{}))
		}
	})

	It("selects environment profiles without inheriting unrelated resource values", func() {
		g := &gateway{Name: "root"}
		root := profile{
			DefaultEnv:   "dev",
			SpaceUID:     "root-space",
			Gateway:      g,
			Environments: map[string]*profile{"dev": {}, "bad": nil},
		}
		selected, err := selectProfile(root, "dev", true)
		Expect(err).NotTo(HaveOccurred())
		Expect(selected.Gateway).To(Equal(g))
		Expect(selected.SpaceUID).To(BeEmpty())
		for _, name := range []string{"bad", "missing"} {
			_, err := selectProfile(root, name, false)
			Expect(err).To(HaveOccurred())
		}
		selected, err = selectProfile(root, "", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(selected.SpaceUID).To(Equal("root-space"))
		Expect(os.Setenv("BK_CLI_MONITOR_SPACE_UID", "env-space")).To(Succeed())
		Expect(root.space("logs", "")).To(Equal("env-space"))
		Expect(root.space("logs", "flag-space")).To(Equal("flag-space"))
	})

	It("records configuration and request failures without making gateway calls", func() {
		out, err := run(newStatusCmd(systemtest.BuildDeps(false)), "--env", "missing")
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring(`"resource": "config"`))
		Expect(out).To(ContainSubstring(`"failed": 1`))
		Expect(os.WriteFile(path, []byte(`{"log_tables":{"empty":""}}`), 0o600)).To(Succeed())
		out, err = run(newStatusCmd(systemtest.BuildDeps(false)))
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring("configure a query space"))
		Expect(out).To(ContainSubstring("--table is required"))
		Expect(out).To(ContainSubstring(`"failed": 2`))
	})

	It("records transport failures for every configured probe", func() {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		server.Close()
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		out, err := run(newStatusCmd(systemtest.BuildDeps(false)), "--env", "dev")
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring(`"failed": 5`))
		Expect(out).To(ContainSubstring(`"verified": false`))
		Expect(out).To(ContainSubstring("connection refused"))
		out, err = run(newQueryCmd("logs", systemtest.BuildDeps(false)))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("connection refused"))
		Expect(out).To(BeEmpty())
	})

	It("propagates output failures for previews, local status and query results", func() {
		for _, cmd := range []*cobra.Command{newQueryCmd("logs", systemtest.BuildDeps(true)), newStatusCmd(systemtest.BuildDeps(true)), newStatusCmd(systemtest.BuildDeps(false)), newEnvsCmd(systemtest.BuildDeps(false))} {
			cmd.SetOut(failingOutput{})
			cmd.SetErr(io.Discard)
			args := []string{"--config", path}
			if cmd.Name() == "status" {
				args = append(args, "--local")
			}
			cmd.SetArgs(args)
			Expect(cmd.Execute()).To(MatchError(ContainSubstring("output failure")))
		}
	})

	DescribeTable(
		"reports reachable query validation errors",
		func(kind string, args []string, message string) {
			out, err := run(newQueryCmd(kind, systemtest.BuildDeps(true)), args...)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(message))
			Expect(out).To(BeEmpty())
		},
		Entry(
			"empty trace field",
			"trace",
			[]string{"--id", "abc", "--trace-field", " "},
			"--trace-field must not be empty",
		),
		Entry("invalid filter", "logs", []string{"--where", "invalid"}, "--where expects FIELD=VALUE"),
		Entry("empty filter name", "logs", []string{"--where", "=x"}, "--where expects FIELD=VALUE"),
		Entry("empty filter value", "logs", []string{"--where", "x="}, "--where expects FIELD=VALUE"),
		Entry(
			"invalid query time",
			"logs",
			[]string{"--end", "invalid"},
			"time must be Unix seconds or RFC3339",
		),
		Entry("missing metric field", "metrics", nil, "--field is required"),
		Entry(
			"invalid metric step",
			"metrics",
			[]string{"--field", "cpu", "--step", "invalid"},
			"--step must be a positive duration",
		),
		Entry(
			"zero metric step",
			"metrics",
			[]string{"--field", "cpu", "--step", "0s"},
			"--step must be a positive duration",
		),
		Entry(
			"ungrouped aggregate",
			"metrics",
			[]string{"--field", "cpu", "--group-by", "pod"},
			"--group-by requires --aggregate",
		),
		Entry("missing query list", "logs", []string{"--body", `{}`}, "non-empty query_list"),
		Entry("empty query list", "logs", []string{"--body", `{"query_list":[]}`}, "non-empty query_list"),
		Entry(
			"missing query times",
			"logs",
			[]string{"--body", `{"query_list":[{}]}`},
			"requires string start_time",
		),
		Entry(
			"wrong time type",
			"logs",
			[]string{"--body", `{"query_list":[{}],"start_time":"1","end_time":2}`},
			"requires string end_time",
		),
		Entry("trailing JSON", "logs", []string{"--body", `{} {}`}, "exactly one JSON object"),
		Entry("empty expression", "query", nil, "--promql is required"),
		Entry("missing body expression", "query", []string{"--body", `{}`}, "PromQL input requires promql"),
		Entry("empty body expression", "query", []string{"--body", `{"promql":" "}`}, "--promql is required"),
		Entry("missing PromQL times", "query", []string{"--body", `{"promql":"up"}`}, "requires string start"),
		Entry(
			"invalid PromQL time",
			"query",
			[]string{"--end", "bad", "--promql", "up"},
			"time must be Unix seconds or RFC3339",
		),
	)
	It("rejects an unconfigured namespace placeholder and accepts equivalent literal queries", func() {
		_, err := expandPromQL(`up{namespace={{namespace}}}`, "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("configure metrics_namespace"))
		for _, quote := range []string{"'", "`"} {
			_, err := expandPromQL("up{namespace="+quote+"{{namespace}}"+quote+"}", "dev")
			Expect(err).To(HaveOccurred())
		}
		expression, err := expandPromQL("vector(1)", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(expression).To(Equal("vector(1)"))
	})
	It("requires an existing context for queries, envs and status", func() {
		Expect(os.Remove(config.ConfigPath("default"))).To(Succeed())
		for _, cmd := range []*cobra.Command{newEnvsCmd(systemtest.BuildDeps(true)), newStatusCmd(systemtest.BuildDeps(true)), newQueryCmd("logs", systemtest.BuildDeps(true))} {
			_, err := run(cmd)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("config"))
		}
	})
	It("propagates successful query and status output errors without losing failure summaries", func() {
		server := httptest.NewServer(
			http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"list":[]}`) },
			),
		)
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		for _, cmd := range []*cobra.Command{newQueryCmd("logs", systemtest.BuildDeps(false)), newStatusCmd(systemtest.BuildDeps(false)), newStatusCmd(systemtest.BuildDeps(true))} {
			cmd.SetOut(failingOutput{})
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{"--config", path})
			Expect(cmd.Execute()).To(MatchError(ContainSubstring("output failure")))
		}
	})
	It("returns verified status after all probes succeed, including empty logs", func() {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if strings.HasSuffix(r.URL.Path, "/query/ts/raw") {
				_, _ = io.WriteString(w, `{"list":[]}`)
			} else {
				_, _ = io.WriteString(w, `{"series":[{"columns":["_value"],"values":[[1]]}]}`)
			}
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		out, err := run(newStatusCmd(systemtest.BuildDeps(false)), "--env", "dev")
		Expect(err).NotTo(HaveOccurred())
		Expect(calls).To(Equal(5))
		Expect(out).To(ContainSubstring(`"verified": true`))
		Expect(out).To(ContainSubstring(`"failed": 0`))
		Expect(out).To(ContainSubstring(`"empty": 2`))
	})
	It("combines metric filters and namespace with AND without changing either", func() {
		request := preview("metrics", "--field", "cpu", "--where", "pod=api", "--aggregate", "sum", "--group-by", "pod, ,namespace")
		q := request["body"].(map[string]any)["query_list"].([]any)[0].(map[string]any)
		conditions := q["conditions"].(map[string]any)
		fields := conditions["field_list"].([]any)
		Expect(fields).To(HaveLen(2))
		Expect(fields[0].(map[string]any)["value"]).To(Equal([]any{"api"}))
		Expect(fields[1].(map[string]any)["value"]).To(Equal([]any{"dev-ns"}))
		Expect(conditions["condition_list"]).To(Equal([]any{"and"}))
		Expect(q["function"].([]any)[0].(map[string]any)["dimensions"]).To(Equal([]any{"pod", "namespace"}))
	})

})

type failingOutput struct{}

func (failingOutput) Write([]byte) (int, error) { return 0, fmt.Errorf("output failure") }
