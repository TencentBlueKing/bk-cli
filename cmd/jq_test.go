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

package cmd

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

var _ = Describe("jq command integration", func() {
	var calls atomic.Int32
	var server *httptest.Server
	BeforeEach(func() {
		calls.Store(0)
		tmp, err := os.MkdirTemp("", "bk-cli-jq-")
		Expect(err).NotTo(HaveOccurred())
		previous, existed := os.LookupEnv("BK_CLI_CONFIG_DIR")
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmp)).To(Succeed())
		DeferCleanup(func() {
			if existed {
				Expect(os.Setenv("BK_CLI_CONFIG_DIR", previous)).To(Succeed())
			} else {
				Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
			}
			Expect(os.RemoveAll(tmp)).To(Succeed())
		})
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			calls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/bk-demo/prod/fail" {
				w.WriteHeader(500)
			}
			_, _ = io.WriteString(
				w,
				`{"id":18446744073709551617,"count":1,"info":[{"bk_host_id":9007199254740993}]}`,
			)
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		rootContext = ""
		rootDryRun = false
		rootVerbose = false
		rootInsecure = false
	})
	DescribeTable(
		"projects precise JSON at all final output boundaries",
		func(args []string, expected string) {
			root := newRootCmd()
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			Expect(executeRoot(root, args)).To(Succeed())
			Expect(stdout.String()).To(Equal(expected))
			Expect(calls.Load()).To(Equal(int32(1)))
		},
		Entry(
			"raw API",
			[]string{"api", "bk-demo", "GET", "/result", "--jq", ".data.id"},
			"18446744073709551617\n",
		),
		Entry(
			"YAML",
			[]string{"apigateway", "list_gateways", "--jq", ".data.id + 1"},
			"18446744073709551618\n",
		),
		Entry(
			"Go action",
			[]string{"cmdb", "search_business", "--body", "{}", "--jq", ".data.id"},
			"18446744073709551617\n",
		),
		Entry(
			"aggregate",
			[]string{
				"cmdb",
				"list_biz_hosts_all",
				"--bk_biz_id",
				"2",
				"--jq",
				".data.info | map(.bk_host_id)",
			},
			"[9007199254740993]\n",
		),
	)
	DescribeTable("validates jq before auth and remote execution", func(expr string) {
		root := newRootCmd()
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		Expect(os.RemoveAll(os.Getenv("BK_CLI_CONFIG_DIR"))).To(Succeed())
		err := executeRoot(root, []string{"api", "bk-demo", "POST", "/result", "--body", "-", "--jq", expr})
		Expect(err).To(MatchError(ContainSubstring("--jq")))
		Expect(calls.Load()).To(BeZero())
	}, Entry("empty", ""), Entry("whitespace", " \n"), Entry("invalid", ".["), Entry("unknown", "missing_function"))
	It("retains completed POST results when projection fails and never resends", func() {
		root := newRootCmd()
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		err := executeRoot(
			root,
			[]string{"api", "bk-demo", "POST", "/result", "--body", "{}", "--jq", "1, .data.id[]"},
		)
		var cliErr *output.CLIError
		Expect(errors.As(err, &cliErr)).To(BeTrue())
		Expect(cliErr.ExitCode).To(Equal(2))
		Expect(calls.Load()).To(Equal(int32(1)))
		Expect(stdout.String()).To(HavePrefix("{\n"))
		Expect(stdout.String()).To(ContainSubstring("18446744073709551617"))
		Expect(stderr.String()).To(ContainSubstring("jq_execution_error"))
		Expect(stderr.String()).To(ContainSubstring("request has already completed"))
	})
	It("does not filter failed HTTP envelopes or change their legacy exit behavior", func() {
		root := newRootCmd()
		var stdout bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(io.Discard)
		Expect(executeRoot(root, []string{"api", "bk-demo", "GET", "/fail", "--jq", "empty"})).To(Succeed())
		Expect(stdout.String()).To(ContainSubstring(`"ok": false`))
		Expect(stdout.String()).To(ContainSubstring(`"status": 500`))
	})
	It("projects redacted dry-run input without losing integer precision", func() {
		root := newRootCmd()
		var stdout bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(io.Discard)
		Expect(
			executeRoot(
				root,
				[]string{
					"api",
					"bk-demo",
					"POST",
					"/result",
					"--body",
					`{"id":18446744073709551617}`,
					"--dry-run",
					"--jq",
					`[.request.body.id,.request.headers["X-Bkapi-Authorization"]]`,
				},
			),
		).To(
			Succeed(),
		)
		Expect(stdout.String()).To(ContainSubstring("18446744073709551617"))
		Expect(stdout.String()).NotTo(ContainSubstring("token-123"))
		Expect(calls.Load()).To(BeZero())
	})
	DescribeTable("cannot expose locally echoed credentials through Go action projection", func(dryRun bool) {
		root := newRootCmd()
		var stdout bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(io.Discard)
		args := []string{
			"apigateway",
			"demo_action",
			"--header",
			"x-bkapi-authorization:secret-test-value",
			"--jq",
			".data.received.headers[0]",
		}
		if dryRun {
			args = append(args, "--dry-run")
		}
		Expect(executeRoot(root, args)).To(Succeed())
		Expect(stdout.String()).To(Equal("\"x-bkapi-authorization:{...redacted...}\"\n"))
	}, Entry("preview", true), Entry("completed request", false))

	It("executes jq only after all pages are aggregated", func() {
		server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			page := calls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if page == 1 {
				_, _ = io.WriteString(
					w,
					`{"count":3,"info":[{"bk_host_id":9007199254740993},{"bk_host_id":9007199254740994}]}`,
				)
			} else {
				_, _ = io.WriteString(w, `{"count":3,"info":[{"bk_host_id":9007199254740995}]}`)
			}
		})
		root := newRootCmd()
		var stdout bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(io.Discard)
		Expect(
			executeRoot(
				root,
				[]string{
					"cmdb",
					"list_biz_hosts_all",
					"--bk_biz_id",
					"2",
					"--page_limit",
					"2",
					"--jq",
					"[.data.count, (.data.info | map(.bk_host_id) | add)]",
				},
			),
		).To(
			Succeed(),
		)
		Expect(calls.Load()).To(Equal(int32(2)))
		Expect(stdout.String()).To(Equal("[3,27021597764222982]\n"))
	})
	It("shares stdin body input with jq without a second read", func() {
		reader, writer, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		_, err = io.WriteString(writer, `{"id":18446744073709551617}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(writer.Close()).To(Succeed())
		original := os.Stdin
		os.Stdin = reader
		DeferCleanup(func() { os.Stdin = original; Expect(reader.Close()).To(Succeed()) })
		root := newRootCmd()
		var stdout bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(io.Discard)
		Expect(
			executeRoot(
				root,
				[]string{
					"api",
					"bk-demo",
					"POST",
					"/result",
					"--body",
					"-",
					"--dry-run",
					"--jq",
					".request.body.id",
				},
			),
		).To(
			Succeed(),
		)
		Expect(stdout.String()).To(Equal("18446744073709551617\n"))
		Expect(calls.Load()).To(BeZero())
	})
	It("leaves help in help mode and rejects jq on unsupported commands", func() {
		root := newRootCmd()
		var stdout bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(io.Discard)
		Expect(executeRoot(root, []string{"api", "--help", "--jq", ".data"})).To(Succeed())
		Expect(stdout.String()).To(ContainSubstring("--jq"))
		Expect(calls.Load()).To(BeZero())
		root = newRootCmd()
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		Expect(
			executeRoot(root, []string{"version", "--jq", "."}),
		).To(
			MatchError(ContainSubstring("unknown flag: --jq")),
		)
	})
})
