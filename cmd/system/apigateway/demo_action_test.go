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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
)

var _ = Describe("apigateway demo_action", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-demo-action-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("registers the expected flags", func() {
		cmd := newDemoActionCmd(systemtest.BuildDeps(true))

		Expect(cmd.Flag("name")).NotTo(BeNil())
		Expect(cmd.Flag("public")).NotTo(BeNil())
		Expect(cmd.Flag("stage")).NotTo(BeNil())
		Expect(cmd.Flag("body")).NotTo(BeNil())
		Expect(cmd.Flag("header")).NotTo(BeNil())
	})

	It("shows received inputs in dry-run output without forwarding local-only fields", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newDemoActionCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("name", "demo")).To(Succeed())
		Expect(cmd.Flags().Set("public", "true")).To(Succeed())
		Expect(cmd.Flags().Set("body", `{"hello":"world"}`)).To(Succeed())
		Expect(cmd.Flags().Set("header", "foo:bar")).To(Succeed())
		Expect(cmd.Flags().Set("stage", "testing")).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())
		Expect(env["dry_run"]).To(BeTrue())

		request := env["request"].(map[string]any)
		Expect(request["url"]).To(Equal(
			"https://bkapi.example.com/api/bk-apigateway/testing/api/v2/open/gateways/",
		))
		Expect(request).NotTo(HaveKey("body"))
		params, ok := request["params"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(params).To(HaveKeyWithValue("name", "demo"))

		headers := request["headers"].(map[string]any)
		Expect(headers["foo"]).To(Equal("bar"))

		data := env["data"].(map[string]any)
		received := data["received"].(map[string]any)
		Expect(received["name"]).To(Equal("demo"))
		Expect(received["public"]).To(BeTrue())
		Expect(received["body"]).To(Equal(`{"hello":"world"}`))
		Expect(received["stage"]).To(Equal("testing"))
		Expect(received["headers"]).To(ContainElement("foo:bar"))
	})

	It("executes the sample upstream request and wraps upstream data with received inputs", func() {
		type capturedRequest struct {
			Method  string
			Path    string
			Query   url.Values
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
				Query:   r.URL.Query(),
				Header:  r.Header.Clone(),
				RawBody: string(body),
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Request-Id", "req-123")
			_, _ = w.Write([]byte(`{"items":[{"name":"bk-iam"}]}`))
		}))
		DeferCleanup(server.Close)

		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())

		cmd := newDemoActionCmd(systemtest.BuildDeps(false))
		Expect(cmd.Flags().Set("name", "demo")).To(Succeed())
		Expect(cmd.Flags().Set("public", "true")).To(Succeed())
		Expect(cmd.Flags().Set("body", `{"hello":"world"}`)).To(Succeed())
		Expect(cmd.Flags().Set("header", "foo:bar")).To(Succeed())
		Expect(cmd.Flags().Set("stage", "testing")).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(captured.Method).To(Equal("GET"))
		Expect(captured.Path).To(Equal("/bk-apigateway/testing/api/v2/open/gateways/"))
		Expect(captured.Query.Get("name")).To(Equal("demo"))
		Expect(captured.Query.Has("public")).To(BeFalse())
		Expect(captured.RawBody).To(BeEmpty())
		Expect(captured.Header.Get("foo")).To(Equal("bar"))

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())
		Expect(env["status"]).To(BeNumerically("==", 200))

		headers := env["headers"].(map[string]any)
		Expect(headers["X-Request-Id"]).To(Equal("req-123"))

		data := env["data"].(map[string]any)
		received := data["received"].(map[string]any)
		Expect(received["name"]).To(Equal("demo"))
		Expect(received["public"]).To(BeTrue())
		Expect(received["body"]).To(Equal(`{"hello":"world"}`))

		upstream := data["upstream"].(map[string]any)
		items := upstream["items"].([]any)
		Expect(items).To(HaveLen(1))
	})
})
