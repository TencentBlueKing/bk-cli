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

package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

var _ = Describe("runAPI", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-api-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())

		cfg := &config.Config{BkAPIURLTmpl: "https://bkapi.example.com/api/{gateway_name}/"}
		Expect(cfg.Save(config.ConfigPath("default"))).To(Succeed())
		Expect(config.SetActiveContext("default")).To(Succeed())

		key, err := credential.DeriveKey()
		Expect(err).NotTo(HaveOccurred())
		cred := &credential.Credential{Type: credential.TypeAccessToken, AccessToken: "token-123"}
		Expect(credential.Save(config.CredentialsPath("default"), cred, key)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("rejects an invalid timeout override before any request", func() {
		err := runAPI(
			bytes.NewBuffer(nil),
			"bk-apigateway",
			"GET",
			"/api/v1/resources/",
			"",
			"",
			"",
			nil,
			"prod",
			"",
			"not-a-duration",
			true,
			false,
			false,
		)
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("request_error"))
		Expect(cliErr.Message).To(ContainSubstring("invalid --timeout value"))
	})

	It("rejects an invalid raw API gateway name before URL generation", func() {
		err := runAPI(
			bytes.NewBuffer(nil),
			"bad_gateway",
			"GET",
			"/api/v1/resources/",
			"",
			"",
			"",
			nil,
			"prod",
			"",
			"",
			true,
			false,
			false,
		)
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("invalid_gateway_name"))
		Expect(cliErr.Message).To(ContainSubstring("gateway_name"))
	})

	It("writes dry-run output to the command writer", func() {
		cmd := NewAPICmd(
			func() string { return "" },
			func() bool { return true },
			func() bool { return false },
			func() bool { return false },
		)
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetArgs([]string{"bk-apigateway", "GET", "/api/v1/resources/"})

		err := cmd.Execute()
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal(stdout.Bytes(), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())
		Expect(env["dry_run"]).To(BeTrue())
	})

	It("documents gateway_name as the first raw API argument", func() {
		cmd := NewAPICmd(
			func() string { return "" },
			func() bool { return false },
			func() bool { return false },
			func() bool { return false },
		)

		Expect(cmd.Use).To(ContainSubstring("<gateway_name>"))
		Expect(cmd.Long).To(ContainSubstring("Render bk_api_url_tmpl with gateway_name"))
		Expect(cmd.Long).NotTo(ContainSubstring("system_name"))
		Expect(cmd.Long).To(
			ContainSubstring("API gateway 403: if X-Bkapi-Error-Code is 1640301"),
		)
		Expect(cmd.Long).To(
			ContainSubstring("bk_error_code 9900403 (IAM permission error)"),
		)
	})

	It("uses the timeout override instead of the context timeout", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(150 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		DeferCleanup(server.Close)

		cfg := &config.Config{
			BkAPIURLTmpl: strings.TrimSuffix(server.URL, "/") + "/{gateway_name}",
			Timeout:      50 * time.Millisecond,
		}
		Expect(cfg.Save(config.ConfigPath("default"))).To(Succeed())

		err := runAPI(
			bytes.NewBuffer(nil),
			"bk-apigateway",
			"GET",
			"/api/v1/resources/",
			"",
			"",
			"",
			nil,
			"prod",
			"",
			"200ms",
			false,
			false,
			false,
		)
		Expect(err).NotTo(HaveOccurred())
	})

	It("uses the context timeout when no override is provided", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(150 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		DeferCleanup(server.Close)

		cfg := &config.Config{
			BkAPIURLTmpl: strings.TrimSuffix(server.URL, "/") + "/{gateway_name}",
			Timeout:      50 * time.Millisecond,
		}
		Expect(cfg.Save(config.ConfigPath("default"))).To(Succeed())

		err := runAPI(
			bytes.NewBuffer(nil),
			"bk-apigateway",
			"GET",
			"/api/v1/resources/",
			"",
			"",
			"",
			nil,
			"prod",
			"",
			"",
			false,
			false,
			false,
		)
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("network_error"))
		Expect(cliErr.Message).To(ContainSubstring("context deadline exceeded"))
	})

	It("returns a local error for invalid --query JSON during dry-run", func() {
		err := runAPI(
			bytes.NewBuffer(nil),
			"bk-apigateway",
			"GET",
			"/api/v1/resources/",
			"",
			`{bad`,
			"",
			nil,
			"prod",
			"",
			"",
			true,
			false,
			false,
		)
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("request_error"))
		Expect(cliErr.Message).To(ContainSubstring("invalid --query JSON"))
	})

	It("returns a local error for invalid --body JSON during dry-run", func() {
		err := runAPI(
			bytes.NewBuffer(nil),
			"bk-apigateway",
			"POST",
			"/api/v1/resources/",
			"",
			"",
			`{bad`,
			nil,
			"prod",
			"",
			"",
			true,
			false,
			false,
		)
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("request_error"))
		Expect(cliErr.Message).To(ContainSubstring("invalid --body JSON"))
	})

	It("returns a local error for malformed --header during dry-run", func() {
		err := runAPI(
			bytes.NewBuffer(nil),
			"bk-apigateway",
			"GET",
			"/api/v1/resources/",
			"",
			"",
			"",
			[]string{"missing-separator"},
			"prod",
			"",
			"",
			true,
			false,
			false,
		)
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("request_error"))
		Expect(cliErr.Message).To(ContainSubstring("invalid --header value"))
	})

	It("lets user-provided auth and tenant headers override generated values during dry-run", func() {
		key, err := credential.DeriveKey()
		Expect(err).NotTo(HaveOccurred())
		cred := &credential.Credential{
			Type:        credential.TypeAccessToken,
			AccessToken: "token-123",
		}
		Expect(credential.Save(config.CredentialsPath("default"), cred, key)).To(Succeed())

		cfg := &config.Config{
			BkAPIURLTmpl: "https://bkapi.example.com/api/{gateway_name}/",
			TenantID:     "context-tenant",
		}
		Expect(cfg.Save(config.ConfigPath("default"))).To(Succeed())

		stdout := &bytes.Buffer{}
		err = runAPI(
			stdout,
			"bk-apigateway",
			"GET",
			"/api/v1/resources/",
			"",
			"",
			"",
			[]string{
				"X-Bkapi-Authorization:custom-auth",
				"X-Bk-Tenant-Id:custom-tenant",
			},
			"prod",
			"",
			"",
			true,
			false,
			false,
		)
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal(stdout.Bytes(), &env)).To(Succeed())
		request := env["request"].(map[string]any)
		headers := request["headers"].(map[string]any)
		Expect(headers).To(HaveKeyWithValue("X-Bkapi-Authorization", "{...redacted...}"))
		Expect(headers).To(HaveKeyWithValue("X-Bk-Tenant-Id", "custom-tenant"))
	})

	It("rejects content-type overrides when a body is provided", func() {
		err := runAPI(
			bytes.NewBuffer(nil),
			"bk-apigateway",
			"POST",
			"/api/v1/resources/",
			"",
			"",
			`{"name":"demo"}`,
			[]string{"Content-Type:text/plain"},
			"prod",
			"",
			"",
			true,
			false,
			false,
		)
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("request_error"))
		Expect(cliErr.Message).To(ContainSubstring("Content-Type"))
	})

	It("uses context tenant_id in dry-run output", func() {
		key, err := credential.DeriveKey()
		Expect(err).NotTo(HaveOccurred())
		cred := &credential.Credential{
			Type:        credential.TypeAccessToken,
			AccessToken: "token-123",
		}
		Expect(credential.Save(config.CredentialsPath("default"), cred, key)).To(Succeed())
		cfg := &config.Config{
			BkAPIURLTmpl: "https://bkapi.example.com/api/{gateway_name}/",
			TenantID:     "context-tenant",
		}
		Expect(cfg.Save(config.ConfigPath("default"))).To(Succeed())

		stdout := &bytes.Buffer{}
		err = runAPI(
			stdout,
			"bk-apigateway",
			"GET",
			"/api/v1/resources/",
			"",
			"",
			"",
			nil,
			"prod",
			"",
			"",
			true,
			false,
			false,
		)
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal(stdout.Bytes(), &env)).To(Succeed())
		request := env["request"].(map[string]any)
		headers := request["headers"].(map[string]any)
		Expect(headers).To(HaveKeyWithValue("X-Bk-Tenant-Id", "context-tenant"))
	})
})
