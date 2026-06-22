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

package system_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	"github.com/TencentBlueKing/bk-cli/internal/system"
)

func setupRuntimeContext(baseURL string, withCredential bool) {
	cfg := &config.Config{
		BkAPIURLTmpl: strings.TrimRight(baseURL, "/") + "/{gateway_name}/",
		UserKey:      "bk_token",
	}
	Expect(cfg.Save(config.ConfigPath("default"))).To(Succeed())
	Expect(config.SetActiveContext("default")).To(Succeed())
	if !withCredential {
		return
	}

	key, err := credential.DeriveKey()
	Expect(err).NotTo(HaveOccurred())
	cred := &credential.Credential{Type: credential.TypeAccessToken, AccessToken: "token-123"}
	Expect(credential.Save(config.CredentialsPath("default"), cred, key)).To(Succeed())
}

func writeLegacyCredentialFile(ctxName, rawJSON string) {
	key, err := credential.DeriveKey()
	Expect(err).NotTo(HaveOccurred())
	encoded, err := credential.Encrypt([]byte(rawJSON), key)
	Expect(err).NotTo(HaveOccurred())
	path := config.CredentialsPath(ctxName)
	Expect(os.MkdirAll(filepath.Dir(path), 0o700)).To(Succeed())
	Expect(os.WriteFile(path, []byte(encoded), 0o600)).To(Succeed())
}

var _ = Describe("ResolveRuntime", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-system-runtime-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("loads the active context config without requiring credentials", func() {
		setupRuntimeContext("https://bkapi.example.com/api", false)

		runtime, err := system.ResolveRuntime("", true, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(runtime.ContextName).To(Equal("default"))
		Expect(runtime.Config.BkAPIURLTmpl).To(Equal("https://bkapi.example.com/api/{gateway_name}/"))
		Expect(runtime.Credential).To(BeNil())
		Expect(runtime.DryRun).To(BeTrue())
	})
})

var _ = Describe("ExecuteRequest", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-system-request-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("builds a dry-run request envelope without printing", func() {
		setupRuntimeContext("https://bkapi.example.com/api", true)

		runtime, err := system.ResolveRuntime("", true, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(runtime.Credential).To(BeNil())

		result, err := system.ExecuteRequest(runtime, system.RequestSpec{
			GatewayName: "bk-apigateway",
			Method:      "GET",
			Path:        "/api/v2/open/gateways/",
			ParamsJSON:  `{"name":"demo"}`,
			Headers:     []string{"foo:bar"},
			Stage:       "testing",
			AuthConfig: &system.AuthConfig{
				AppVerifiedRequired: true,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(runtime.Credential).NotTo(BeNil())
		Expect(result.DryRunRequest).NotTo(BeNil())
		Expect(result.Envelope.DryRun).To(BeTrue())
		Expect(result.DryRunRequest.URL).To(Equal(
			"https://bkapi.example.com/api/bk-apigateway/testing/api/v2/open/gateways/",
		))

		params, ok := result.DryRunRequest.Params.(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(params["name"]).To(Equal("demo"))
	})

	It("returns an error when authConfig is missing", func() {
		setupRuntimeContext("https://bkapi.example.com/api", true)

		runtime, err := system.ResolveRuntime("", true, false)
		Expect(err).NotTo(HaveOccurred())

		_, err = system.ExecuteRequest(runtime, system.RequestSpec{
			GatewayName: "bk-apigateway",
			Method:      "GET",
			Path:        "/api/v2/open/gateways/",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("request authConfig is required"))
	})

	It("defers auth_required until a request is executed", func() {
		setupRuntimeContext("https://bkapi.example.com/api", false)

		runtime, err := system.ResolveRuntime("", true, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(runtime.Credential).To(BeNil())

		_, err = system.ExecuteRequest(runtime, system.RequestSpec{
			GatewayName: "bk-apigateway",
			Method:      "GET",
			Path:        "/api/v2/open/gateways/",
			AuthConfig: &system.AuthConfig{
				AppVerifiedRequired: true,
			},
		})
		Expect(err).To(HaveOccurred())

		var cliErr *output.CLIError
		Expect(errors.As(err, &cliErr)).To(BeTrue())
		Expect(cliErr.Code).To(Equal("auth_required"))
	})

	It("does not require stored credentials when authConfig requires no auth", func() {
		setupRuntimeContext("https://bkapi.example.com/api", false)

		runtime, err := system.ResolveRuntime("", true, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(runtime.Credential).To(BeNil())

		result, err := system.ExecuteRequest(runtime, system.RequestSpec{
			GatewayName: "bk-apigateway",
			Method:      "GET",
			Path:        "/api/v2/open/gateways/",
			AuthConfig:  &system.AuthConfig{},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(runtime.Credential).To(BeNil())
		Expect(result.DryRunRequest.Headers).NotTo(HaveKey("X-Bkapi-Authorization"))
	})

	It("executes a single upstream API call and returns the response envelope", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		DeferCleanup(server.Close)

		setupRuntimeContext(server.URL, true)

		runtime, err := system.ResolveRuntime("", false, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(runtime.Credential).To(BeNil())

		result, err := system.ExecuteRequest(runtime, system.RequestSpec{
			GatewayName: "bk-apigateway",
			Method:      "GET",
			Path:        "/api/v2/open/gateways/",
			AuthConfig: &system.AuthConfig{
				AppVerifiedRequired: true,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(runtime.Credential).NotTo(BeNil())
		Expect(result.DryRunRequest).To(BeNil())
		Expect(result.Envelope.Status).To(Equal(200))
		Expect(result.Envelope.OK).To(BeTrue())
	})

	It("writes request and response details to stderr when verbose is enabled", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Request-Id", "req-123")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		DeferCleanup(server.Close)

		setupRuntimeContext(server.URL, true)

		runtime, err := system.ResolveRuntime("", false, true)
		Expect(err).NotTo(HaveOccurred())

		origStderr := os.Stderr
		r, w, pipeErr := os.Pipe()
		Expect(pipeErr).NotTo(HaveOccurred())
		os.Stderr = w

		_, err = system.ExecuteRequest(runtime, system.RequestSpec{
			GatewayName: "bk-apigateway",
			Method:      "GET",
			Path:        "/api/v2/open/gateways/",
			Headers:     []string{"X-Debug:true"},
			AuthConfig: &system.AuthConfig{
				AppVerifiedRequired: true,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(w.Close()).To(Succeed())
		os.Stderr = origStderr

		stderr, readErr := io.ReadAll(r)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(r.Close()).To(Succeed())

		output := string(stderr)
		Expect(output).To(ContainSubstring("> GET "))
		Expect(output).To(ContainSubstring("> X-Bkapi-Authorization: {redacted}"))
		Expect(output).To(ContainSubstring("> X-Debug: true"))
		Expect(output).To(ContainSubstring("< 200 200 OK"))
		Expect(output).To(ContainSubstring("< X-Request-Id: req-123"))
	})

	It("ignores tenant_id stored in legacy credentials when the context does not define one", func() {
		setupRuntimeContext("https://bkapi.example.com/api", false)
		writeLegacyCredentialFile(
			"default",
			`{"type":"access_token","access_token":"token-123","tenant_id":"legacy-tenant"}`,
		)

		runtime, err := system.ResolveRuntime("", true, false)
		Expect(err).NotTo(HaveOccurred())

		result, err := system.ExecuteRequest(runtime, system.RequestSpec{
			GatewayName: "bk-apigateway",
			Method:      "GET",
			Path:        "/api/v2/open/gateways/",
			AuthConfig: &system.AuthConfig{
				AppVerifiedRequired: true,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.DryRunRequest.Headers).NotTo(HaveKey("X-Bk-Tenant-Id"))
	})

	It("lets user-provided auth and tenant headers override generated values", func() {
		setupRuntimeContext("https://bkapi.example.com/api", false)

		runtime, err := system.ResolveRuntime("", true, false)
		Expect(err).NotTo(HaveOccurred())
		runtime.Credential = &credential.Credential{
			Type:        credential.TypeAccessToken,
			AccessToken: "token-123",
		}
		runtime.Config.TenantID = "context-tenant"

		result, err := system.ExecuteRequest(runtime, system.RequestSpec{
			GatewayName: "bk-apigateway",
			Method:      "GET",
			Path:        "/api/v2/open/gateways/",
			Headers: []string{
				"X-Bkapi-Authorization:custom-auth",
				"X-Bk-Tenant-Id:custom-tenant",
			},
			AuthConfig: &system.AuthConfig{
				AppVerifiedRequired: true,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.DryRunRequest.Headers).To(HaveKeyWithValue("X-Bkapi-Authorization", "{...redacted...}"))
		Expect(result.DryRunRequest.Headers).To(HaveKeyWithValue("X-Bk-Tenant-Id", "custom-tenant"))
	})
})
