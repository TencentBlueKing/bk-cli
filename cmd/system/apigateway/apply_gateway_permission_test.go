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
