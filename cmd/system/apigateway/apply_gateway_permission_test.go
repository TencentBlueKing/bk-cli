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

package apigateway

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
	"github.com/spf13/cobra"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

var _ = Describe("apigateway apply_gateway_permission", func() {
	const body = `{"target_app_code":"my-app","grant_dimension":"resource","resource_names":["user_query","user_create"],"applicant":"developer","reason":"Integration","expire_days":180}`
	var requests atomic.Int32
	var capturedBody, capturedPath, capturedMethod string
	var capturedHeader http.Header
	var response string
	var status int
	var server *httptest.Server

	newCommand := func(dryRun bool) *cobra.Command {
		parent := &cobra.Command{Use: "apigateway"}
		Expect(NewSystemSpec().RegisterGoActions(parent, systemtest.BuildDeps(dryRun))).To(Succeed())
		for _, child := range parent.Commands() {
			if child.Name() == "apply_gateway_permission" {
				return child
			}
		}
		Fail("apply_gateway_permission must be registered in the existing apigateway system")
		return nil
	}

	BeforeEach(func() {
		original, present := os.LookupEnv("BK_CLI_CONFIG_DIR")
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", GinkgoT().TempDir())).To(Succeed())
		DeferCleanup(func() {
			if present {
				Expect(os.Setenv("BK_CLI_CONFIG_DIR", original)).To(Succeed())
			} else {
				Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
			}
		})
		requests.Store(0)
		capturedBody, capturedPath, capturedMethod = "", "", ""
		response = `{"code":0,"message":"OK","data":{"record_id":12345,"approval_url":"https://approval.example.com/12345","approvers":["reviewer"]}}`
		status = http.StatusOK
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			raw, _ := io.ReadAll(r.Body)
			capturedBody, capturedPath, capturedMethod = string(raw), r.URL.Path, r.Method
			capturedHeader = r.Header.Clone()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(response))
		}))
		DeferCleanup(server.Close)
	})

	It("requires confirmation before resolving credentials or reading body files", func() {
		cmd := newCommand(false)
		Expect(cmd.Flags().Set("gateway_name", "bk-iam")).To(Succeed())
		Expect(cmd.Flags().Set("body", "@missing.json")).To(Succeed())
		err := cmd.RunE(cmd, nil)
		Expect(err).To(MatchError(ContainSubstring("confirmation_required")))
		Expect(requests.Load()).To(BeZero())
	})

	DescribeTable("refuses every submission without explicit user confirmation",
		func(args []string) {
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			cmd := newCommand(false)
			args = append(args, "--gateway_name", "bk-iam", "--body", body)
			Expect(cmd.ParseFlags(args)).To(Succeed())
			err := cmd.RunE(cmd, nil)
			Expect(err).To(MatchError(ContainSubstring("confirmation_required")))
			Expect(requests.Load()).To(BeZero())
		},
		Entry("no confirmation flag", []string{}),
		Entry("explicit refusal", []string{"--confirmed=false"}),
	)

	It("documents mandatory confirmation, batch scope, and both body dimensions", func() {
		cmd := newCommand(false)
		var help bytes.Buffer
		cmd.SetOut(&help)
		Expect(cmd.Help()).To(Succeed())
		Expect(help.String()).To(ContainSubstring("must ask the user"))
		Expect(help.String()).To(ContainSubstring("every submission"))
		Expect(help.String()).To(ContainSubstring("cannot be skipped"))
		Expect(help.String()).To(ContainSubstring("Batch"))
		Expect(help.String()).To(ContainSubstring("target_app_code"))
		Expect(help.String()).To(ContainSubstring("resource_names"))
		Expect(help.String()).To(ContainSubstring("grant_dimension"))
		Expect(help.String()).To(ContainSubstring("applicant"))
		Expect(help.String()).To(ContainSubstring("expire_days"))
		Expect(cmd.Flag("yes")).To(BeNil())
	})

	It("previews an unconfirmed batch without sending a request or claiming submission", func() {
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		cmd := newCommand(true)
		Expect(cmd.Flags().Set("gateway_name", "bk-iam")).To(Succeed())
		Expect(cmd.Flags().Set("body", body)).To(Succeed())
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		Expect(cmd.RunE(cmd, nil)).To(Succeed())
		var env output.Envelope
		Expect(json.Unmarshal(stdout.Bytes(), &env)).To(Succeed())
		Expect(env.DryRun).To(BeTrue())
		Expect(env.Message).To(BeEmpty())
		Expect(env.Request.Method).To(Equal("POST"))
		Expect(
			env.Request.URL,
		).To(
			HaveSuffix("/bk-apigateway/prod/api/v2/open/gateways/bk-iam/permissions/apply/"),
		)
		Expect(env.Request.Headers["X-Bkapi-Authorization"]).To(Equal("{...redacted...}"))
		Expect(env.Request.Body).To(HaveKeyWithValue("resource_names", []any{"user_query", "user_create"}))
		Expect(requests.Load()).To(BeZero())
	})

	DescribeTable("rejects personal token authentication before preview or submission",
		func(dryRun, personalContext bool, headers []string) {
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			if personalContext {
				key, err := credential.DeriveKey()
				Expect(err).NotTo(HaveOccurred())
				Expect(credential.Save(config.CredentialsPath("default"), &credential.Credential{
					Type: credential.TypePersonalToken, PersonalToken: "test-personal-token",
				}, key)).To(Succeed())
			}
			cmd := newCommand(dryRun)
			args := []string{"--gateway_name", "bk-iam", "--body", body}
			if !dryRun {
				args = append(args, "--confirmed")
			}
			for _, header := range headers {
				args = append(args, "--header", header)
			}
			Expect(cmd.ParseFlags(args)).To(Succeed())
			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			err := cmd.RunE(cmd, nil)
			Expect(err).To(MatchError(ContainSubstring("unsupported_credential_type")))
			Expect(err.Error()).To(ContainSubstring("application identity"))
			Expect(err.Error()).NotTo(ContainSubstring("test-personal-token"))
			Expect(stdout.String()).To(BeEmpty())
			Expect(requests.Load()).To(BeZero())
		},
		Entry("personal context submission", false, true, nil),
		Entry("personal context preview", true, true, nil),
		Entry("personal context with app header override", false, true,
			[]string{`x-bkapi-authorization:{"access_token":"app-token"}`}),
		Entry("app context with Bearer header submission", false, false,
			[]string{"authorization:bEaReR test-personal-token"}),
		Entry("app context with Bearer header preview", true, false,
			[]string{"Authorization:Bearer test-personal-token"}),
	)

	DescribeTable("checks command-local personal token restrictions for cached credentials and header overrides",
		func(credType credential.CredentialType, headers map[string]string, rejected bool) {
			var cred *credential.Credential
			if credType != "" {
				cred = &credential.Credential{Type: credType}
			}
			err := validatePersonalTokenAuth(cred, headers)
			if rejected {
				Expect(err).To(MatchError(ContainSubstring("unsupported_credential_type")))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("anonymous without headers", credential.CredentialType(""), nil, false),
		Entry("application credentials", credential.TypeAppUser, nil, false),
		Entry("access token credentials", credential.TypeAccessToken, nil, false),
		Entry("cached personal credentials", credential.TypePersonalToken, nil, true),
		Entry("explicit Bearer header without stored credentials", credential.CredentialType(""),
			map[string]string{"aUtHoRiZaTiOn": " bEaReR personal"}, true),
		Entry("non-Bearer header", credential.TypeAccessToken,
			map[string]string{"Authorization": "Basic example"}, false),
		Entry("empty header", credential.TypeAccessToken,
			map[string]string{"Authorization": ""}, false),
	)

	DescribeTable("submits exactly one batch and preserves all returned approval details",
		func(input string) {
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			bodyInput := body
			if input == "file" {
				file := filepath.Join(GinkgoT().TempDir(), "body.json")
				Expect(os.WriteFile(file, []byte(body), 0o600)).To(Succeed())
				bodyInput = "@" + file
			} else if input == "stdin" {
				file, err := os.CreateTemp(GinkgoT().TempDir(), "stdin")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(file.Close)
				_, err = file.WriteString(body)
				Expect(err).NotTo(HaveOccurred())
				_, err = file.Seek(0, 0)
				Expect(err).NotTo(HaveOccurred())
				original := os.Stdin
				os.Stdin = file
				DeferCleanup(func() { os.Stdin = original })
				bodyInput = "-"
			}
			cmd := newCommand(false)
			Expect(cmd.ParseFlags([]string{
				"--confirmed", "--gateway_name", "bk-iam", "--body", bodyInput,
				"--stage", "testing", "--header", "X-Custom:batch",
			})).To(Succeed())
			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			Expect(cmd.RunE(cmd, nil)).To(Succeed())
			Expect(requests.Load()).To(Equal(int32(1)))
			Expect(capturedMethod).To(Equal("POST"))
			Expect(
				capturedPath,
			).To(
				Equal("/bk-apigateway/testing/api/v2/open/gateways/bk-iam/permissions/apply/"),
			)
			Expect(capturedBody).To(MatchJSON(body))
			Expect(capturedHeader.Get("X-Custom")).To(Equal("batch"))
			Expect(capturedHeader.Get("X-Bkapi-Authorization")).NotTo(BeEmpty())
			var env output.Envelope
			Expect(json.Unmarshal(stdout.Bytes(), &env)).To(Succeed())
			Expect(env.OK).To(BeTrue())
			Expect(env.Message).To(ContainSubstring("12345"))
			Expect(env.Message).To(ContainSubstring("does not mean permission has been granted"))
			Expect(
				env.Data,
			).To(
				HaveKeyWithValue(
					"data",
					HaveKeyWithValue("approval_url", "https://approval.example.com/12345"),
				),
			)
			Expect(env.Data).To(HaveKeyWithValue("data", HaveKeyWithValue("approvers", []any{"reviewer"})))
		},
		Entry("inline JSON", "inline"),
		Entry("body file", "file"),
		Entry("stdin body", "stdin"),
	)

	DescribeTable("rejects invalid local inputs before contacting the gateway",
		func(gateway, input, errorCode string) {
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			cmd := newCommand(false)
			Expect(
				cmd.ParseFlags([]string{"--confirmed", "--gateway_name", gateway, "--body", input}),
			).To(
				Succeed(),
			)
			Expect(cmd.RunE(cmd, nil)).To(MatchError(ContainSubstring(errorCode)))
			Expect(requests.Load()).To(BeZero())
		},
		Entry("missing gateway", "", body, "invalid_argument"),
		Entry("gateway path injection", "bk-iam/other", body, "invalid_gateway_name"),
		Entry("missing body", "bk-iam", "", "invalid_argument"),
		Entry("blank body", "bk-iam", "  ", "invalid_argument"),
		Entry("invalid JSON", "bk-iam", "{", "request_error"),
	)

	DescribeTable(
		"delegates permission fields and body structure to the server",
		func(input string) {
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			status = http.StatusBadRequest
			response = `{"code":1,"message":"Server rejected application fields"}`
			cmd := newCommand(false)
			Expect(
				cmd.ParseFlags([]string{"--confirmed", "--gateway_name", "bk-iam", "--body", input}),
			).To(
				Succeed(),
			)
			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			Expect(cmd.RunE(cmd, nil)).To(Succeed())
			Expect(requests.Load()).To(Equal(int32(1)))
			Expect(capturedBody).To(Equal(input))
			var env output.Envelope
			Expect(json.Unmarshal(stdout.Bytes(), &env)).To(Succeed())
			Expect(env.OK).To(BeFalse())
			Expect(env.Status).To(Equal(http.StatusBadRequest))
			Expect(env.Data).To(HaveKeyWithValue("message", "Server rejected application fields"))
			Expect(env.Message).To(BeEmpty())
		},
		Entry("array body", `[]`),
		Entry("null body", `null`),
		Entry("string body", `"application"`),
		Entry("boolean body", `true`),
		Entry("numeric body", `42`),
		Entry("missing required fields", `{}`),
		Entry(
			"blank target application",
			`{"target_app_code":" ","grant_dimension":"gateway","applicant":"developer"}`,
		),
		Entry(
			"numeric target application",
			`{"target_app_code":12,"grant_dimension":"gateway","applicant":"developer"}`,
		),
		Entry("blank applicant", `{"target_app_code":"my-app","grant_dimension":"gateway","applicant":" "}`),
		Entry("null applicant", `{"target_app_code":"my-app","grant_dimension":"gateway","applicant":null}`),
		Entry(
			"unknown dimension",
			`{"target_app_code":"my-app","grant_dimension":"resouce","applicant":"developer"}`,
		),
		Entry(
			"missing resources",
			`{"target_app_code":"my-app","grant_dimension":"resource","applicant":"developer"}`,
		),
		Entry(
			"empty resources",
			`{"target_app_code":"my-app","grant_dimension":"resource","applicant":"developer","resource_names":[]}`,
		),
		Entry(
			"string resources",
			`{"target_app_code":"my-app","grant_dimension":"resource","applicant":"developer","resource_names":"api"}`,
		),
		Entry(
			"blank resource",
			`{"target_app_code":"my-app","grant_dimension":"resource","applicant":"developer","resource_names":[" "]}`,
		),
		Entry(
			"null resource",
			`{"target_app_code":"my-app","grant_dimension":"resource","applicant":"developer","resource_names":[null]}`,
		),
		Entry(
			"numeric resource",
			`{"target_app_code":"my-app","grant_dimension":"resource","applicant":"developer","resource_names":[1]}`,
		),
		Entry(
			"invalid expiry",
			`{"target_app_code":"my-app","grant_dimension":"gateway","applicant":"developer","expire_days":90}`,
		),
		Entry(
			"fractional expiry",
			`{"target_app_code":"my-app","grant_dimension":"gateway","applicant":"developer","expire_days":180.5}`,
		),
		Entry(
			"string expiry",
			`{"target_app_code":"my-app","grant_dimension":"gateway","applicant":"developer","expire_days":"180"}`,
		),
		Entry(
			"null expiry",
			`{"target_app_code":"my-app","grant_dimension":"gateway","applicant":"developer","expire_days":null}`,
		),
	)

	DescribeTable("rejects invalid JSON from inline, file and stdin during preview and submission",
		func(source string, dryRun bool) {
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			input := `{`
			bodyInput := input
			if source == "file" {
				path := filepath.Join(GinkgoT().TempDir(), "body.json")
				Expect(os.WriteFile(path, []byte(input), 0o600)).To(Succeed())
				bodyInput = "@" + path
			} else if source == "stdin" {
				r, w, err := os.Pipe()
				Expect(err).NotTo(HaveOccurred())
				original := os.Stdin
				os.Stdin = r
				DeferCleanup(func() { os.Stdin = original; Expect(r.Close()).To(Succeed()) })
				_, err = w.WriteString(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(w.Close()).To(Succeed())
				bodyInput = "-"
			}
			cmd := newCommand(dryRun)
			Expect(
				cmd.ParseFlags(
					[]string{"--confirmed", "--gateway_name", "bk-iam", "--body", bodyInput},
				),
			).To(
				Succeed(),
			)
			Expect(cmd.RunE(cmd, nil)).To(MatchError(ContainSubstring("--body JSON")))
			Expect(requests.Load()).To(BeZero())
		},
		Entry("inline preview", "inline", true),
		Entry("inline submission", "inline", false),
		Entry("file submission", "file", false),
		Entry("stdin submission", "stdin", false),
		Entry("file preview", "file", true),
		Entry("stdin preview", "stdin", true),
	)

	DescribeTable(
		"keeps application bodies unchanged as the server contract evolves",
		func(input string) {
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			cmd := newCommand(false)
			Expect(
				cmd.ParseFlags([]string{"--confirmed", "--gateway_name", "bk-iam", "--body", input}),
			).To(
				Succeed(),
			)
			Expect(cmd.RunE(cmd, nil)).To(Succeed())
			Expect(requests.Load()).To(Equal(int32(1)))
			Expect(capturedBody).To(Equal(input))
		},
		Entry(
			"legacy dimension and string expiry",
			`{"target_app_code":"my-app","grant_dimension":"api","applicant":"developer","expire_days":"180"}`,
		),
		Entry(
			"future dimension and expiry",
			`{"grant_dimension":"future","expire_days":90,"future_field":{"id":9007199254740993}}`,
		),
		Entry(
			"gateway with omitted expiry",
			`{"target_app_code":"my-app","grant_dimension":"gateway","applicant":"developer"}`,
		),
		Entry(
			"permanent gateway",
			`{"target_app_code":"my-app","grant_dimension":"gateway","applicant":"developer","expire_days":0}`,
		),
		Entry(
			"twelve month batch with extra upstream field",
			`{"target_app_code":"my-app","grant_dimension":"resource","applicant":"developer","expire_days":360,"resource_names":["api1","api2"],"extra":{"id":9007199254740993}}`,
		),
	)

	DescribeTable(
		"only reports submission when the upstream returns an application record",
		func(upstream, recordID string) {
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			response = upstream
			cmd := newCommand(false)
			Expect(
				cmd.ParseFlags([]string{"--confirmed", "--gateway_name", "bk-iam", "--body", body}),
			).To(
				Succeed(),
			)
			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			Expect(cmd.RunE(cmd, nil)).To(Succeed())
			var env output.Envelope
			Expect(json.Unmarshal(stdout.Bytes(), &env)).To(Succeed())
			if recordID != "" {
				Expect(env.Message).To(ContainSubstring("record_id: " + recordID + ")"))
				if upstream == `{"data":{"record_id":9007199254740993}}` ||
					upstream == `{"data":{"record_id":18446744073709551617}}` {
					var raw struct {
						Data struct {
							Data struct {
								RecordID json.RawMessage `json:"record_id"`
							} `json:"data"`
						} `json:"data"`
					}
					Expect(json.Unmarshal(stdout.Bytes(), &raw)).To(Succeed())
					Expect(string(raw.Data.Data.RecordID)).To(Equal(recordID))
				}

			} else {
				Expect(env.Message).To(BeEmpty())
			}
			Expect(requests.Load()).To(Equal(int32(1)))
		},
		Entry("documented response without optional approval details", `{"data":{"record_id":12345}}`, "12345"),
		Entry(
			"large numeric record ID without scientific notation",
			`{"data":{"record_id":1234567}}`,
			"1234567",
		),
		Entry(
			"string record ID preserved verbatim",
			`{"data":{"record_id":"9007199254740993"}}`,
			"9007199254740993",
		),
		Entry(
			"numeric ID beyond float64 integer precision",
			`{"data":{"record_id":9007199254740993}}`,
			"9007199254740993",
		),
		Entry(
			"numeric ID beyond int64 range",
			`{"data":{"record_id":18446744073709551617}}`,
			"18446744073709551617",
		),
		Entry("business error returned with HTTP 200", `{"code":1,"message":"Rejected"}`, ""),
		Entry("unexpected response shape", `[]`, ""),
	)

	It("preserves upstream errors without a successful application message", func() {
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		status = http.StatusForbidden
		response = `{"code":1640301,"message":"Permission denied"}`
		cmd := newCommand(false)
		Expect(
			cmd.ParseFlags([]string{"--confirmed", "--gateway_name", "bk-iam", "--body", body}),
		).To(
			Succeed(),
		)
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		Expect(cmd.RunE(cmd, nil)).To(Succeed())
		var env output.Envelope
		Expect(json.Unmarshal(stdout.Bytes(), &env)).To(Succeed())
		Expect(env.OK).To(BeFalse())
		Expect(env.Status).To(Equal(http.StatusForbidden))
		Expect(env.Message).To(BeEmpty())
		Expect(env.Data).To(HaveKeyWithValue("message", "Permission denied"))
		Expect(requests.Load()).To(Equal(int32(1)))
	})
})
