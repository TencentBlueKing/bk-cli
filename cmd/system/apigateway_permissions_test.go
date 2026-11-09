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

package system

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

var _ = Describe("apigateway list_current_app_permissions", func() {
	const response = `{"data":{"count":30,"results":[
		{"id":9007199254740993,"bk_app_code":"my-app","gateway_name":"bk-demo",
		 "resource_name":"","expires":null,"grant_type":"initialize"},
		{"id":2,"bk_app_code":"my-app","gateway_name":"bk-demo",
		 "resource_name":"user_query","expires":"2020-01-01 00:00:00 +0800","grant_type":"apply"}
	]}}`

	buildCommand := func(dryRun bool) *cobra.Command {
		root := &cobra.Command{Use: "bk-cli"}
		Expect(registerSystemSpecs(root, []systemcmd.SystemSpec{newApigatewaySystemSpec()},
			systemtest.BuildDeps(dryRun), actionsFS)).To(Succeed())
		cmd, _, err := root.Find([]string{"apigateway", "list_current_app_permissions"})
		Expect(err).NotTo(HaveOccurred())
		Expect(cmd.Name()).To(Equal("list_current_app_permissions"))
		return cmd
	}

	BeforeEach(func() {
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", GinkgoT().TempDir())
	})

	DescribeTable("sends one current-app query and preserves the complete upstream page",
		func(flags map[string]string, query url.Values) {
			var captured *http.Request
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured = r.Clone(r.Context())
				calls++
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(response))
			}))
			DeferCleanup(server.Close)
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			cmd := buildCommand(false)
			for name, value := range flags {
				Expect(cmd.Flags().Set(name, value)).To(Succeed())
			}
			Expect(cmd.Flags().Set("stage", "testing")).To(Succeed())
			Expect(cmd.Flags().Set("header", "X-Request-Id:permissions-test")).To(Succeed())
			stdout, err := systemtest.CaptureCommandStdout(func() error { return cmd.RunE(cmd, nil) })
			Expect(err).NotTo(HaveOccurred())
			Expect(calls).To(Equal(1))
			Expect(captured.Method).To(Equal(http.MethodGet))
			Expect(captured.URL.Path).To(Equal("/bk-apigateway/testing/api/v2/open/app/permissions/"))
			Expect(captured.URL.Query()).To(Equal(query))
			Expect(captured.Header.Get("X-Request-Id")).To(Equal("permissions-test"))
			Expect(
				captured.Header.Get("X-Bkapi-Authorization"),
			).To(
				MatchJSON(`{"access_token":"token-123"}`),
			)
			var envelope output.Envelope
			Expect(json.Unmarshal([]byte(stdout), &envelope)).To(Succeed())
			Expect(envelope.OK).To(BeTrue())
			Expect(envelope.Status).To(Equal(200))
			Expect(stdout).To(ContainSubstring("9007199254740993"))
			// A page must retain expired and permanent records and its total count.
			var upstream any
			Expect(json.Unmarshal([]byte(response), &upstream)).To(Succeed())
			Expect(envelope.Data).To(Equal(upstream))
		},
		Entry("gateway dimension with server pagination defaults",
			map[string]string{"grant_dimension": "gateway"},
			url.Values{"grant_dimension": {"gateway"}}),
		Entry("resource dimension with exact filters and explicit pagination",
			map[string]string{
				"grant_dimension": "resource", "gateway_name": "bk-demo",
				"resource_name": "user query/+", "limit": "10", "offset": "10",
			},
			url.Values{
				"grant_dimension": {"resource"}, "gateway_name": {"bk-demo"},
				"resource_name": {"user query/+"}, "limit": {"10"}, "offset": {"10"},
			}),
		Entry("explicit zero pagination values are passed to the upstream",
			map[string]string{"grant_dimension": "resource", "limit": "0", "offset": "0"},
			url.Values{"grant_dimension": {"resource"}, "limit": {"0"}, "offset": {"0"}}),
	)

	DescribeTable("uses shared authentication without permission-application restrictions",
		func(cred *credential.Credential, expectedHeader, expectedValue string) {
			var headers http.Header
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				headers = r.Header.Clone()
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(response))
			}))
			DeferCleanup(server.Close)
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			key, err := credential.DeriveKey()
			Expect(err).NotTo(HaveOccurred())
			Expect(credential.Save(config.CredentialsPath("default"), cred, key)).To(Succeed())
			cmd := buildCommand(false)
			Expect(cmd.Flags().Set("grant_dimension", "gateway")).To(Succeed())
			_, err = systemtest.CaptureCommandStdout(func() error { return cmd.RunE(cmd, nil) })
			Expect(err).NotTo(HaveOccurred())
			Expect(headers.Get(expectedHeader)).To(Equal(expectedValue))
		},
		Entry(
			"app identity without sending user credentials",
			&credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "my-app",
				BkAppSecret: "test-secret",
				BkToken:     "test-user",
			},
			"X-Bkapi-Authorization",
			`{"bk_app_code":"my-app","bk_app_secret":"test-secret"}`,
		),
		Entry("personal identity resolved by the server",
			&credential.Credential{Type: credential.TypePersonalToken, PersonalToken: "test-personal"},
			"Authorization", "Bearer test-personal"),
	)

	It("rejects a missing dimension before sending a request", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com")).To(Succeed())
		cmd := buildCommand(false)
		err := cmd.RunE(cmd, nil)
		var cliErr *output.CLIError
		Expect(err).To(BeAssignableToTypeOf(cliErr))
		Expect(err.(*output.CLIError).Code).To(Equal("missing_param"))
		Expect(err.Error()).To(ContainSubstring("--grant_dimension"))
	})

	It("previews a redacted request and supports jq without contacting the upstream", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com")).To(Succeed())
		cmd := buildCommand(true)
		Expect(cmd.Flags().Set("grant_dimension", "resource")).To(Succeed())
		Expect(
			cmd.Flags().Set(
				"header",
				`X-Bkapi-Authorization:{"bk_app_code":"my-app","bk_app_secret":"private-secret"}`,
			),
		).To(
			Succeed(),
		)
		Expect(
			cmd.Flags().Set(
				"jq",
				`{dry_run,method:.request.method,params:.request.params,auth:.request.headers["X-Bkapi-Authorization"]}`,
			),
		).To(
			Succeed(),
		)
		stdout, err := systemtest.CaptureCommandStdout(func() error { return cmd.RunE(cmd, nil) })
		Expect(err).NotTo(HaveOccurred())
		Expect(stdout).NotTo(ContainSubstring("private-secret"))
		Expect(stdout).NotTo(ContainSubstring("token-123"))
		Expect(stdout).To(MatchJSON(`{"dry_run":true,"method":"GET","params":{"grant_dimension":"resource"},
			"auth":"{...redacted...}"}`))
	})

	It("preserves failed upstream envelopes when jq is supplied", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"permission denied"}`))
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		cmd := buildCommand(false)
		Expect(cmd.Flags().Set("grant_dimension", "resource")).To(Succeed())
		Expect(cmd.Flags().Set("jq", ".data.data.results")).To(Succeed())
		stdout, err := systemtest.CaptureCommandStdout(func() error { return cmd.RunE(cmd, nil) })
		Expect(err).NotTo(HaveOccurred())
		var envelope output.Envelope
		Expect(json.Unmarshal([]byte(stdout), &envelope)).To(Succeed())
		Expect(envelope.OK).To(BeFalse())
		Expect(envelope.Status).To(Equal(403))
		Expect(stdout).To(ContainSubstring("permission denied"))
	})

	It("documents pagination and permission interpretation in help without requiring a context", func() {
		cmd := buildCommand(false)
		Expect(cmd.Flag("target_app_code")).To(BeNil())
		Expect(cmd.Flag("limit").DefValue).To(Equal("10"))
		Expect(cmd.Flag("offset").DefValue).To(Equal("0"))
		var help bytes.Buffer
		cmd.SetOut(&help)
		Expect(cmd.Help()).To(Succeed())
		for _, text := range []string{
			"current authenticated app", "single page", "count", "offset", "expires",
			"null", "expired", "gateway grants", "resource_name is empty", "--grant_dimension gateway",
			"--grant_dimension resource", "--gateway_name bk-demo", "--resource_name user_query", "--jq",
		} {
			Expect(help.String()).To(ContainSubstring(text))
		}
	})
})
