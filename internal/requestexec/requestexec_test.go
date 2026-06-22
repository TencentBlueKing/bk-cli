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

package requestexec

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/api"
	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

func writeRequestExecContext(name, baseURL string, withCredential bool) {
	cfg := &config.Config{
		BkAPIURLTmpl: strings.TrimRight(baseURL, "/") + "/{gateway_name}/",
		TenantID:     "tenant-from-context",
	}
	Expect(cfg.Save(config.ConfigPath(name))).To(Succeed())
	Expect(config.SetActiveContext(name)).To(Succeed())

	if !withCredential {
		return
	}

	key, err := credential.DeriveKey()
	Expect(err).NotTo(HaveOccurred())

	cred := &credential.Credential{
		Type:        credential.TypeAccessToken,
		AccessToken: "token-123",
	}
	Expect(credential.Save(config.CredentialsPath(name), cred, key)).To(Succeed())
}

func expectCLIError(err error, code string) *output.CLIError {
	Expect(err).To(HaveOccurred())

	var cliErr *output.CLIError
	Expect(errors.As(err, &cliErr)).To(BeTrue())
	Expect(cliErr.Code).To(Equal(code))

	return cliErr
}

var _ = Describe("requestexec", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-requestexec-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	Describe("ResolveRuntime", func() {
		It("loads the active context config without requiring credentials", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", false)

			runtime, err := ResolveRuntime("", true, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(runtime.ContextName).To(Equal("default"))
			Expect(runtime.Config.BkAPIURLTmpl).To(Equal("https://bkapi.example.com/api/{gateway_name}/"))
			Expect(runtime.Credential).To(BeNil())
			Expect(runtime.DryRun).To(BeTrue())
			Expect(runtime.Verbose).To(BeFalse())
		})

		It("wraps missing context configuration as a user error", func() {
			_, err := ResolveRuntime("", false, true)

			cliErr := expectCLIError(err, "config_error")
			Expect(cliErr.Message).To(ContainSubstring("no context configured"))
		})
	})

	Describe("loadCredential", func() {
		It("loads stored credentials for a context", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", true)

			cred, err := loadCredential("default")
			Expect(err).NotTo(HaveOccurred())
			Expect(cred.Type).To(Equal(credential.TypeAccessToken))
			Expect(cred.AccessToken).To(Equal("token-123"))
		})

		It("returns auth_required when credentials are missing", func() {
			_, err := loadCredential("missing")

			cliErr := expectCLIError(err, "auth_required")
			Expect(cliErr.Message).To(ContainSubstring("No credentials found for context"))
		})
	})

	Describe("ensureRuntimeCredential", func() {
		It("does nothing when the credential is already present", func() {
			preloaded := &credential.Credential{
				Type:        credential.TypeAccessToken,
				AccessToken: "preloaded-token",
			}
			runtime := &Runtime{
				ContextName: "default",
				Credential:  preloaded,
			}

			err := ensureRuntimeCredential(runtime)
			Expect(err).NotTo(HaveOccurred())
			Expect(runtime.Credential).To(BeIdenticalTo(preloaded))
		})

		It("requires a context name when loading lazily", func() {
			err := ensureRuntimeCredential(&Runtime{})

			Expect(err).To(MatchError("runtime context name is required when credential is not preloaded"))
		})

		It("loads and stores credentials on the runtime", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", true)
			runtime := &Runtime{ContextName: "default"}

			err := ensureRuntimeCredential(runtime)
			Expect(err).NotTo(HaveOccurred())
			Expect(runtime.Credential).NotTo(BeNil())
			Expect(runtime.Credential.AccessToken).To(Equal("token-123"))
		})
	})

	Describe("helper functions", func() {
		It("rejects a Content-Type override when a JSON body is present", func() {
			err := validateUserHeaders(map[string]string{"Content-Type": "text/plain"}, `{"ok":true}`)

			Expect(err).To(MatchError(`header "Content-Type" cannot be overridden when --body is provided`))
		})

		It("allows non-conflicting user headers", func() {
			err := validateUserHeaders(map[string]string{"X-Request-Id": "demo"}, `{"ok":true}`)
			Expect(err).NotTo(HaveOccurred())

			err = validateUserHeaders(map[string]string{"Content-Type": "text/plain"}, "")
			Expect(err).NotTo(HaveOccurred())
		})

		It("resolves tenant IDs from config only", func() {
			Expect(resolveTenantID(nil)).To(BeEmpty())
			Expect(resolveTenantID(&config.Config{TenantID: "tenant-1"})).To(Equal("tenant-1"))
		})

		DescribeTable(
			"resolveTimeout",
			func(defaultTimeout time.Duration, override, label string, expected time.Duration, expectedErr string) {
				timeout, err := resolveTimeout(defaultTimeout, override, label)
				if expectedErr != "" {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedErr))
					return
				}

				Expect(err).NotTo(HaveOccurred())
				Expect(timeout).To(Equal(expected))
			},
			Entry(
				"uses the package default when nothing is configured",
				time.Duration(0),
				"",
				rawAPITimeoutErrorLabel,
				config.DefaultTimeout,
				"",
			),
			Entry(
				"uses the configured default when no override is provided",
				45*time.Second,
				"",
				rawAPITimeoutErrorLabel,
				45*time.Second,
				"",
			),
			Entry(
				"uses an explicit valid override",
				45*time.Second,
				"2m",
				rawAPITimeoutErrorLabel,
				2*time.Minute,
				"",
			),
			Entry(
				"reports raw API timeout parsing errors with the raw flag label",
				time.Second,
				"bad",
				rawAPITimeoutErrorLabel,
				time.Duration(0),
				"invalid --timeout value",
			),
			Entry(
				"falls back to the action label when no label is supplied",
				time.Second,
				"bad",
				"",
				time.Duration(0),
				"invalid action timeout",
			),
			Entry(
				"rejects non-positive timeout overrides",
				time.Second,
				"0s",
				rawAPITimeoutErrorLabel,
				time.Duration(0),
				"must be greater than 0",
			),
		)
	})

	Describe("ExecuteRequest", func() {
		It("requires authConfig for system actions", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", true)

			runtime, err := ResolveRuntime("", true, false)
			Expect(err).NotTo(HaveOccurred())

			_, err = ExecuteRequest(runtime, RequestSpec{
				GatewayName: "bk-apigateway",
				Method:      http.MethodGet,
				Path:        "/api/v1/demo/",
			})
			Expect(err).To(MatchError("request authConfig is required"))
		})

		It("defers credential loading until an authenticated request is executed", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", false)

			runtime, err := ResolveRuntime("", true, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(runtime.Credential).To(BeNil())

			_, err = ExecuteRequest(runtime, RequestSpec{
				GatewayName: "bk-apigateway",
				Method:      http.MethodGet,
				Path:        "/api/v1/demo/",
				AuthConfig: &api.AuthRequirements{
					AppVerifiedRequired: true,
				},
			})

			cliErr := expectCLIError(err, "auth_required")
			Expect(cliErr.Message).To(ContainSubstring("No credentials found"))
		})

		It("skips credential loading when the action requires no auth", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", false)

			runtime, err := ResolveRuntime("", true, false)
			Expect(err).NotTo(HaveOccurred())

			result, err := ExecuteRequest(runtime, RequestSpec{
				GatewayName: "bk-apigateway",
				Method:      http.MethodGet,
				Path:        "/api/v1/demo/",
				AuthConfig:  &api.AuthRequirements{},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(runtime.Credential).To(BeNil())
			Expect(result.Envelope.DryRun).To(BeTrue())
			Expect(result.DryRunRequest.Headers).NotTo(HaveKey("X-Bkapi-Authorization"))
		})

		It("uses the action timeout label for invalid overrides", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", false)

			runtime, err := ResolveRuntime("", true, false)
			Expect(err).NotTo(HaveOccurred())

			_, err = ExecuteRequest(runtime, RequestSpec{
				GatewayName: "bk-apigateway",
				Method:      http.MethodGet,
				Path:        "/api/v1/demo/",
				Timeout:     "bad",
				AuthConfig:  &api.AuthRequirements{},
			})

			cliErr := expectCLIError(err, "request_error")
			Expect(cliErr.Message).To(ContainSubstring("invalid action timeout"))
		})

		It("rejects user Content-Type overrides when a body is provided", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", false)

			runtime, err := ResolveRuntime("", true, false)
			Expect(err).NotTo(HaveOccurred())

			_, err = ExecuteRequest(runtime, RequestSpec{
				GatewayName: "bk-apigateway",
				Method:      http.MethodPost,
				Path:        "/api/v1/demo/",
				BodyJSON:    `{"name":"demo"}`,
				Headers:     []string{"Content-Type:text/plain"},
				AuthConfig:  &api.AuthRequirements{},
			})

			cliErr := expectCLIError(err, "request_error")
			Expect(cliErr.Message).To(ContainSubstring("cannot be overridden"))
		})

		It("builds a dry-run request and lazily loads credentials when auth is required", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", true)

			runtime, err := ResolveRuntime("", true, false)
			Expect(err).NotTo(HaveOccurred())

			result, err := ExecuteRequest(runtime, RequestSpec{
				GatewayName: "bk-apigateway",
				Method:      http.MethodPost,
				Path:        "/api/v1/demo/",
				ParamsJSON:  `{"page":1}`,
				BodyJSON:    `{"name":"demo"}`,
				Headers:     []string{"X-Custom:true"},
				Stage:       "testing",
				AuthConfig: &api.AuthRequirements{
					AppVerifiedRequired: true,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(runtime.Credential).NotTo(BeNil())
			Expect(result.Envelope.DryRun).To(BeTrue())
			Expect(
				result.DryRunRequest.URL,
			).To(
				Equal("https://bkapi.example.com/api/bk-apigateway/testing/api/v1/demo/"),
			)
			Expect(
				result.DryRunRequest.Headers,
			).To(
				HaveKeyWithValue("X-Bkapi-Authorization", "{...redacted...}"),
			)
			Expect(
				result.DryRunRequest.Headers,
			).To(
				HaveKeyWithValue("X-Bk-Tenant-Id", "tenant-from-context"),
			)
			Expect(result.DryRunRequest.Headers).To(HaveKeyWithValue("X-Custom", "true"))
			Expect(result.DryRunRequest.Body).To(Equal(map[string]any{"name": "demo"}))

			params, ok := result.DryRunRequest.Params.(map[string]any)
			Expect(ok).To(BeTrue())
			page, ok := params["page"].(json.Number)
			Expect(ok).To(BeTrue())
			Expect(page.String()).To(Equal("1"))
		})

		It("rewrites bk-job for legacy templates", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", false)
			restore := config.SetBKTeDomainForTesting("te.example")
			DeferCleanup(restore)

			runtime, err := ResolveRuntime("", true, false)
			Expect(err).NotTo(HaveOccurred())
			runtime.Config.BkAPIURLTmpl = "https://{gateway_name}.apigw.te.example"

			result, err := ExecuteRequest(runtime, RequestSpec{
				GatewayName: "bk-job",
				Method:      http.MethodGet,
				Path:        "/api/v3/get_job_instance_status",
				AuthConfig:  &api.AuthRequirements{},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.DryRunRequest.URL).To(Equal(
				"https://jobv3-cloud.apigw.te.example/prod/api/v3/get_job_instance_status",
			))
		})

		It("rewrites bkpaas3 for legacy templates", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", false)
			restore := config.SetBKTeDomainForTesting("te.example")
			DeferCleanup(restore)

			runtime, err := ResolveRuntime("", true, false)
			Expect(err).NotTo(HaveOccurred())
			runtime.Config.BkAPIURLTmpl = "https://{gateway_name}.apigw.te.example"

			result, err := ExecuteRequest(runtime, RequestSpec{
				GatewayName: "bkpaas3",
				Method:      http.MethodGet,
				Path:        "/bkapps/applications/bk-demo/",
				AuthConfig:  &api.AuthRequirements{},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.DryRunRequest.URL).To(Equal(
				"https://paasv3.apigw.te.example/prod/bkapps/applications/bk-demo/",
			))
		})

		It("executes an upstream request and returns the response envelope", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodPost))
				Expect(r.URL.Path).To(Equal("/bk-apigateway/prod/api/v1/demo/"))
				Expect(r.URL.Query().Get("page")).To(Equal("1"))
				Expect(r.Header.Get("X-Bk-Tenant-Id")).To(Equal("tenant-from-context"))
				Expect(
					r.Header.Get("X-Bkapi-Authorization"),
				).To(
					ContainSubstring(`"access_token":"token-123"`),
				)
				Expect(r.Header.Get("X-Custom")).To(Equal("true"))
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Request-Id", "req-123")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"created":true}`))
			}))
			DeferCleanup(server.Close)

			writeRequestExecContext("default", server.URL, true)

			runtime, err := ResolveRuntime("", false, false)
			Expect(err).NotTo(HaveOccurred())

			result, err := ExecuteRequest(runtime, RequestSpec{
				GatewayName: "bk-apigateway",
				Method:      http.MethodPost,
				Path:        "/api/v1/demo/",
				ParamsJSON:  `{"page":1}`,
				BodyJSON:    `{"name":"demo"}`,
				Headers:     []string{"X-Custom:true"},
				AuthConfig: &api.AuthRequirements{
					AppVerifiedRequired: true,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.DryRunRequest).To(BeNil())
			Expect(result.Envelope.OK).To(BeTrue())
			Expect(result.Envelope.Status).To(Equal(http.StatusCreated))
			Expect(result.Envelope.Headers).To(HaveKeyWithValue("X-Request-Id", "req-123"))
			Expect(result.Envelope.Data).To(Equal(map[string]any{"created": true}))
		})

		It("can execute HTTPS requests with self-signed certificates when insecure is enabled", func() {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			DeferCleanup(server.Close)

			writeRequestExecContext("default", server.URL, false)

			runtime, err := ResolveRuntimeWithOptions("", false, false, RuntimeOptions{
				Insecure: true,
			})
			Expect(err).NotTo(HaveOccurred())

			result, err := ExecuteRequest(runtime, RequestSpec{
				GatewayName: "bk-apigateway",
				Method:      http.MethodGet,
				Path:        "/api/v1/demo/",
				AuthConfig:  &api.AuthRequirements{},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Envelope.OK).To(BeTrue())
			Expect(result.Envelope.Data).To(Equal(map[string]any{"ok": true}))
		})

		It("logs verbose request and response details with redacted auth", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Request-Id", "req-verbose")
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			DeferCleanup(server.Close)

			writeRequestExecContext("default", server.URL, true)

			runtime, err := ResolveRuntime("", false, true)
			Expect(err).NotTo(HaveOccurred())

			origStderr := os.Stderr
			r, w, pipeErr := os.Pipe()
			Expect(pipeErr).NotTo(HaveOccurred())
			os.Stderr = w

			_, err = ExecuteRequest(runtime, RequestSpec{
				GatewayName: "bk-apigateway",
				Method:      http.MethodGet,
				Path:        "/api/v1/demo/",
				Headers:     []string{"X-Debug:true"},
				AuthConfig: &api.AuthRequirements{
					AppVerifiedRequired: true,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(w.Close()).To(Succeed())
			os.Stderr = origStderr

			stderr, readErr := io.ReadAll(r)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(r.Close()).To(Succeed())

			logged := string(stderr)
			Expect(logged).To(ContainSubstring("> GET "))
			Expect(logged).To(ContainSubstring("> X-Bkapi-Authorization: {redacted}"))
			Expect(logged).To(ContainSubstring("> X-Debug: true"))
			Expect(logged).To(ContainSubstring("< 200 200 OK"))
			Expect(logged).To(ContainSubstring("< X-Request-Id: req-verbose"))
		})
	})

	Describe("ExecuteRawAPIRequest", func() {
		It("requires a runtime", func() {
			_, err := ExecuteRawAPIRequest(nil, RequestSpec{})

			Expect(err).To(MatchError("runtime is required"))
		})

		It("requires a runtime config", func() {
			_, err := ExecuteRawAPIRequest(&Runtime{}, RequestSpec{})

			Expect(err).To(MatchError("runtime config is required"))
		})

		It("uses the raw API timeout label for invalid overrides", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", true)

			runtime, err := ResolveRuntime("", true, false)
			Expect(err).NotTo(HaveOccurred())

			_, err = ExecuteRawAPIRequest(runtime, RequestSpec{
				GatewayName: "bk-apigateway",
				Method:      http.MethodGet,
				Path:        "/api/v1/demo/",
				Timeout:     "bad",
			})

			cliErr := expectCLIError(err, "request_error")
			Expect(cliErr.Message).To(ContainSubstring("invalid --timeout value"))
		})

		It("rejects malformed header flags", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", true)

			runtime, err := ResolveRuntime("", true, false)
			Expect(err).NotTo(HaveOccurred())

			_, err = ExecuteRawAPIRequest(runtime, RequestSpec{
				GatewayName: "bk-apigateway",
				Method:      http.MethodGet,
				Path:        "/api/v1/demo/",
				Headers:     []string{"missing-separator"},
			})

			cliErr := expectCLIError(err, "request_error")
			Expect(cliErr.Message).To(ContainSubstring("invalid --header value"))
		})

		It("validates gateway names before URL generation", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", true)

			runtime, err := ResolveRuntime("", true, false)
			Expect(err).NotTo(HaveOccurred())

			_, err = ExecuteRawAPIRequest(runtime, RequestSpec{
				GatewayName: "bad_gateway",
				Method:      http.MethodGet,
				Path:        "/api/v1/demo/",
			})

			cliErr := expectCLIError(err, "invalid_gateway_name")
			Expect(cliErr.Message).To(ContainSubstring("gateway_name"))
		})

		It("rewrites raw api requests when the gateway name is bk-job", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", true)
			restore := config.SetBKTeDomainForTesting("te.example")
			DeferCleanup(restore)

			runtime, err := ResolveRuntime("", true, false)
			Expect(err).NotTo(HaveOccurred())
			runtime.Config.BkAPIURLTmpl = "https://{gateway_name}.apigw.te.example"

			result, err := ExecuteRawAPIRequest(runtime, RequestSpec{
				GatewayName: "bk-job",
				Method:      http.MethodGet,
				Path:        "/api/v3/get_job_instance_status",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.DryRunRequest.URL).To(Equal(
				"https://jobv3-cloud.apigw.te.example/prod/api/v3/get_job_instance_status",
			))
		})

		It("returns a request error for invalid JSON bodies", func() {
			writeRequestExecContext("default", "https://bkapi.example.com/api", true)

			runtime, err := ResolveRuntime("", true, false)
			Expect(err).NotTo(HaveOccurred())

			_, err = ExecuteRawAPIRequest(runtime, RequestSpec{
				GatewayName: "bk-apigateway",
				Method:      http.MethodPost,
				Path:        "/api/v1/demo/",
				BodyJSON:    `{bad`,
			})

			cliErr := expectCLIError(err, "request_error")
			Expect(cliErr.Message).To(ContainSubstring("invalid --body JSON"))
		})

		It("returns a network error when the upstream request times out", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(150 * time.Millisecond)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			DeferCleanup(server.Close)

			writeRequestExecContext("default", server.URL, true)

			runtime, err := ResolveRuntime("", false, false)
			Expect(err).NotTo(HaveOccurred())
			runtime.Config.Timeout = 50 * time.Millisecond

			_, err = ExecuteRawAPIRequest(runtime, RequestSpec{
				GatewayName: "bk-apigateway",
				Method:      http.MethodGet,
				Path:        "/api/v1/demo/",
			})

			cliErr := expectCLIError(err, "network_error")
			Expect(cliErr.Message).To(ContainSubstring("context deadline exceeded"))
		})
	})
})
