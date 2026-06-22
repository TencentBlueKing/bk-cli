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

package cmdb

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
)

var _ = Describe("cmdb search_business", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-cmdb-search-business-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("registers the expected flags", func() {
		cmd := newSearchBusinessCmd(systemtest.BuildDeps(true))

		Expect(cmd.Flag("bk_biz_id")).NotTo(BeNil())
		Expect(cmd.Flag("bk_biz_ids")).NotTo(BeNil())
		Expect(cmd.Flag("fields")).NotTo(BeNil())
		Expect(cmd.Flag("limit")).NotTo(BeNil())
		Expect(cmd.Flag("supplier_account")).NotTo(BeNil())
		Expect(cmd.Flag("stage")).NotTo(BeNil())
		Expect(cmd.Flag("body")).NotTo(BeNil())
		Expect(cmd.Flag("header")).NotTo(BeNil())
	})

	It("shows the synthesized dry-run request body", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newSearchBusinessCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("bk_biz_id", "2")).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())
		Expect(env["dry_run"]).To(BeTrue())

		request := env["request"].(map[string]any)
		Expect(request["url"]).To(Equal("https://bkapi.example.com/api/bk-cmdb/prod/api/v3/open/biz/search/0/"))

		body := request["body"].(map[string]any)
		Expect(body["fields"]).To(Equal([]any{
			"bk_biz_id",
			"bk_biz_name",
			"bk_biz_maintainer",
			"bk_biz_productor",
		}))
		page := body["page"].(map[string]any)
		Expect(page["start"]).To(BeNumerically("==", 0))
		Expect(page["limit"]).To(BeNumerically("==", 500))
		condition := body["condition"].(map[string]any)
		Expect(condition["bk_biz_id"]).To(BeNumerically("==", 2))
	})

	It("builds a condition from bk_biz_id", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newSearchBusinessCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("bk_biz_id", "5")).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		body := env["request"].(map[string]any)["body"].(map[string]any)
		condition := body["condition"].(map[string]any)
		Expect(condition["bk_biz_id"]).To(BeNumerically("==", 5))
	})

	It("builds an $in condition from bk_biz_ids", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newSearchBusinessCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("bk_biz_ids", "1, 2,3")).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		body := env["request"].(map[string]any)["body"].(map[string]any)
		condition := body["condition"].(map[string]any)
		bizID := condition["bk_biz_id"].(map[string]any)
		Expect(bizID["$in"]).To(Equal([]any{float64(1), float64(2), float64(3)}))
	})

	It("uses explicit body without merging synthesized fields", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newSearchBusinessCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("bk_biz_id", "5")).To(Succeed())
		Expect(cmd.Flags().Set("fields", "bk_biz_id")).To(Succeed())
		Expect(cmd.Flags().Set("limit", "10")).To(Succeed())
		Expect(cmd.Flags().Set("supplier_account", "tencent")).To(Succeed())
		Expect(cmd.Flags().Set("body", `{"hello":"world"}`)).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		request := env["request"].(map[string]any)
		Expect(request["url"]).To(Equal(
			"https://bkapi.example.com/api/bk-cmdb/prod/api/v3/open/biz/search/tencent/",
		))
		body := request["body"].(map[string]any)
		Expect(body).To(Equal(map[string]any{"hello": "world"}))
	})

	It("executes the upstream request with the expected request shape", func() {
		type capturedRequest struct {
			Method  string
			Path    string
			Header  http.Header
			RawBody string
		}

		var captured capturedRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())
			captured = capturedRequest{
				Method:  r.Method,
				Path:    r.URL.Path,
				Header:  r.Header.Clone(),
				RawBody: string(body),
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Request-Id", "req-456")
			_, _ = w.Write([]byte(`{"info":[{"bk_biz_id":2}]}`))
		}))
		DeferCleanup(server.Close)

		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())

		cmd := newSearchBusinessCmd(systemtest.BuildDeps(false))
		Expect(cmd.Flags().Set("bk_biz_ids", "1,2,3")).To(Succeed())
		Expect(cmd.Flags().Set("fields", "bk_biz_id,bk_biz_name")).To(Succeed())
		Expect(cmd.Flags().Set("limit", "200")).To(Succeed())
		Expect(cmd.Flags().Set("supplier_account", "tencent")).To(Succeed())
		Expect(cmd.Flags().Set("header", "foo:bar")).To(Succeed())
		Expect(cmd.Flags().Set("stage", "testing")).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(captured.Method).To(Equal("POST"))
		Expect(captured.Path).To(Equal("/bk-cmdb/testing/api/v3/open/biz/search/tencent/"))
		Expect(captured.Header.Get("foo")).To(Equal("bar"))
		Expect(captured.Header.Get("Content-Type")).To(Equal("application/json"))
		Expect(captured.Header.Get("X-Bkapi-Authorization")).NotTo(BeEmpty())

		var requestBody map[string]any
		Expect(json.Unmarshal([]byte(captured.RawBody), &requestBody)).To(Succeed())
		Expect(requestBody["fields"]).To(Equal([]any{"bk_biz_id", "bk_biz_name"}))
		page := requestBody["page"].(map[string]any)
		Expect(page["start"]).To(BeNumerically("==", 0))
		Expect(page["limit"]).To(BeNumerically("==", 200))
		condition := requestBody["condition"].(map[string]any)
		bizID := condition["bk_biz_id"].(map[string]any)
		Expect(bizID["$in"]).To(Equal([]any{float64(1), float64(2), float64(3)}))

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())
		Expect(env["status"]).To(BeNumerically("==", 200))
		headers := env["headers"].(map[string]any)
		Expect(headers["X-Request-Id"]).To(Equal("req-456"))
	})

	It("returns a local error for invalid bk_biz_id", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newSearchBusinessCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("bk_biz_id", "0")).To(Succeed())

		err := cmd.RunE(cmd, nil)
		Expect(err).To(MatchError(ContainSubstring("bk_biz_id must be greater than 0")))
	})

	It("returns a local error for invalid bk_biz_ids", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newSearchBusinessCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("bk_biz_ids", "1, two")).To(Succeed())

		err := cmd.RunE(cmd, nil)
		Expect(err).To(MatchError(ContainSubstring("bk_biz_ids must be a comma-separated list of integers")))
	})

	It("returns a local error for invalid limit", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newSearchBusinessCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("limit", "0")).To(Succeed())

		err := cmd.RunE(cmd, nil)
		Expect(err).To(MatchError(ContainSubstring("limit must be greater than 0")))
	})

	It("requires at least one of bk_biz_id or bk_biz_ids when body is synthesized", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newSearchBusinessCmd(systemtest.BuildDeps(true))

		err := cmd.RunE(cmd, nil)
		Expect(err).To(MatchError(ContainSubstring(
			"one of bk_biz_id or bk_biz_ids is required when --body is not provided",
		)))
	})

	It("allows explicit body without bk_biz_id or bk_biz_ids", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newSearchBusinessCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("body", `{"hello":"world"}`)).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		request := env["request"].(map[string]any)
		body := request["body"].(map[string]any)
		Expect(body).To(Equal(map[string]any{"hello": "world"}))
	})

	It("returns a local error for blank supplier_account", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newSearchBusinessCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("supplier_account", "   ")).To(Succeed())

		err := cmd.RunE(cmd, nil)
		Expect(err).To(MatchError(ContainSubstring("supplier_account cannot be empty")))
	})

	It("enforces mutual exclusion for bk_biz_id and bk_biz_ids", func() {
		cmd := newSearchBusinessCmd(systemtest.BuildDeps(true))
		cmd.SetArgs([]string{"--bk_biz_id", "1", "--bk_biz_ids", "1,2"})
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true

		err := cmd.Execute()
		Expect(err).To(MatchError(ContainSubstring(
			"if any flags in the group [bk_biz_id bk_biz_ids] are set none of the others can be",
		)))
	})
})
