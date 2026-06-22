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
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	"github.com/TencentBlueKing/bk-cli/internal/system"
)

func buildActionCmd(action *system.Action) *cobra.Command {
	inputSpec, err := system.BuildActionInputSpec(action)
	Expect(err).NotTo(HaveOccurred())

	cmd := &cobra.Command{Use: action.Name}
	parent := &cobra.Command{Use: "test-system"}
	parent.AddCommand(cmd)
	cmd.Flags().String(system.ActionStageFlagName, "prod", "API gateway stage")
	cmd.Flags().String(system.ActionBodyFlagName, "", "JSON request body")
	cmd.Flags().StringArray(system.ActionHeaderFlagName, nil, "Additional headers (key:value, repeatable)")

	for _, flag := range inputSpec.GeneratedFlags {
		p := flag.Param
		switch p.Type {
		case "bool":
			cmd.Flags().Bool(flag.FlagName, p.Default == "true", p.Description)
		case "int":
			cmd.Flags().Int(flag.FlagName, 0, p.Description)
		default:
			cmd.Flags().String(flag.FlagName, p.Default, p.Description)
		}
	}

	return cmd
}

func buildNestedActionCmd(action *system.Action) *cobra.Command {
	inputSpec, err := system.BuildActionInputSpec(action)
	Expect(err).NotTo(HaveOccurred())

	cmd := &cobra.Command{Use: action.Name}
	root := &cobra.Command{Use: "bk-cli"}
	systemCmd := &cobra.Command{Use: "devops"}
	subsystemCmd := &cobra.Command{Use: "pipeline"}
	root.AddCommand(systemCmd)
	systemCmd.AddCommand(subsystemCmd)
	subsystemCmd.AddCommand(cmd)
	cmd.Flags().String(system.ActionStageFlagName, "prod", "API gateway stage")
	cmd.Flags().String(system.ActionBodyFlagName, "", "JSON request body")
	cmd.Flags().StringArray(system.ActionHeaderFlagName, nil, "Additional headers (key:value, repeatable)")
	system.RegisterActionFlags(cmd, inputSpec)
	return cmd
}

func captureStdout(fn func() error) (string, error) {
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w

	runErr := fn()

	_ = w.Close()
	os.Stdout = origStdout

	out, readErr := io.ReadAll(r)
	_ = r.Close()
	Expect(readErr).NotTo(HaveOccurred())
	return string(out), runErr
}

func captureStderr(fn func() error) (string, error) {
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stderr = w

	runErr := fn()

	_ = w.Close()
	os.Stderr = origStderr

	out, readErr := io.ReadAll(r)
	_ = r.Close()
	Expect(readErr).NotTo(HaveOccurred())
	return string(out), runErr
}

func requiredAuthConfig() *system.AuthConfig {
	return &system.AuthConfig{AppVerifiedRequired: true}
}

var _ = Describe("RunAction", func() {
	var (
		cfg     *config.Config
		cred    *credential.Credential
		runtime *system.Runtime
	)

	BeforeEach(func() {
		cfg = &config.Config{
			BkAPIURLTmpl: "https://bkapi.example.com/api/{gateway_name}/",
			TenantID:     "tenant-1",
		}
		cred = &credential.Credential{Type: credential.TypeAccessToken, AccessToken: "token-123"}
		runtime = &system.Runtime{
			Config:     cfg,
			Credential: cred,
			DryRun:     true,
		}
	})

	It("routes path, query, body, and header inputs to the correct request fields", func() {
		action := &system.Action{
			Name:       "list_resources",
			Method:     "POST",
			Path:       "/api/v1/{gateway}/resources/{resource_id}/",
			AuthConfig: requiredAuthConfig(),
			Params: []system.Param{
				{Name: "gateway", In: "path", Type: "string", Required: true},
				{Name: "resource_id", In: "path", Type: "int", Required: true},
				{Name: "keyword", In: "query", Type: "string"},
				{Name: "fuzzy", In: "query", Type: "bool"},
			},
		}
		inputSpec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())

		cmd := buildActionCmd(action)
		Expect(cmd.Flags().Set("gateway", "bk-iam")).To(Succeed())
		Expect(cmd.Flags().Set("resource_id", "42")).To(Succeed())
		Expect(cmd.Flags().Set("keyword", "search")).To(Succeed())
		Expect(cmd.Flags().Set("fuzzy", "true")).To(Succeed())
		Expect(cmd.Flags().Set(system.ActionBodyFlagName, `{"name":"svc-a","enabled":true}`)).To(Succeed())
		Expect(cmd.Flags().Set(system.ActionHeaderFlagName, "X-Request-Token:req-123")).To(Succeed())
		Expect(cmd.Flags().Set(system.ActionHeaderFlagName, "X-Debug:true")).To(Succeed())

		stdout, err := captureStdout(func() error {
			return system.RunAction(action, inputSpec, "bk-apigateway", cmd, runtime, "prod")
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())
		Expect(env["dry_run"]).To(BeTrue())

		request, ok := env["request"].(map[string]any)
		Expect(ok).To(BeTrue())
		body, ok := request["body"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(body).To(HaveKeyWithValue("name", "svc-a"))
		Expect(body).To(HaveKeyWithValue("enabled", true))
		Expect(
			request["url"],
		).To(
			Equal("https://bkapi.example.com/api/bk-apigateway/prod/api/v1/bk-iam/resources/42/"),
		)

		headers, ok := request["headers"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(headers["X-Request-Token"]).To(Equal("req-123"))
		Expect(headers["X-Debug"]).To(Equal("true"))
		Expect(headers["X-Bk-Tenant-Id"]).To(Equal("tenant-1"))
		Expect(headers["X-Bkapi-Authorization"]).To(Equal("{...redacted...}"))
		Expect(headers["Content-Type"]).To(Equal("application/json"))

		params, ok := request["params"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(params["keyword"]).To(Equal("search"))
		Expect(params["fuzzy"]).To(BeTrue())
		Expect(params).NotTo(HaveKey("gateway"))
		Expect(params).NotTo(HaveKey("resource_id"))
	})

	It("preserves large integer query params in executed YAML actions", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal(http.MethodGet))
			Expect(r.URL.Path).To(Equal("/api/bk-job/prod/api/v3/get_job_instance_status"))
			Expect(r.URL.Query().Get("bk_biz_id")).To(Equal("2"))
			Expect(r.URL.Query().Get("job_instance_id")).To(Equal("20004841045"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		DeferCleanup(server.Close)

		runtime.DryRun = false
		runtime.Config.BkAPIURLTmpl = server.URL + "/api/{gateway_name}/"

		action := &system.Action{
			Name:       "get_job_instance_status",
			Method:     "GET",
			Path:       "/api/v3/get_job_instance_status",
			AuthConfig: requiredAuthConfig(),
			Params: []system.Param{
				{Name: "bk_biz_id", In: "query", Type: "int", Required: true},
				{Name: "job_instance_id", In: "query", Type: "int", Required: true},
			},
		}
		inputSpec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())

		cmd := buildActionCmd(action)
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		Expect(cmd.Flags().Set("bk_biz_id", "2")).To(Succeed())
		Expect(cmd.Flags().Set("job_instance_id", "20004841045")).To(Succeed())

		err = system.RunAction(action, inputSpec, "bk-job", cmd, runtime, "prod")
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal(stdout.Bytes(), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())
	})

	It("rewrites the job gateway for legacy templates in dry-run YAML actions", func() {
		restore := config.SetBKTeDomainForTesting("te.example")
		DeferCleanup(restore)
		runtime.Config.BkAPIURLTmpl = "https://{gateway_name}.apigw.te.example"

		action := &system.Action{
			Name:       "get_job_instance_status",
			Method:     "GET",
			Path:       "/api/v3/get_job_instance_status",
			AuthConfig: requiredAuthConfig(),
			Params: []system.Param{
				{Name: "bk_biz_id", In: "query", Type: "int", Required: true},
				{Name: "job_instance_id", In: "query", Type: "int", Required: true},
			},
		}
		inputSpec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())

		cmd := buildActionCmd(action)
		Expect(cmd.Flags().Set("bk_biz_id", "2")).To(Succeed())
		Expect(cmd.Flags().Set("job_instance_id", "100")).To(Succeed())

		stdout, err := captureStdout(func() error {
			return system.RunAction(action, inputSpec, "bk-job", cmd, runtime, "prod")
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		request := env["request"].(map[string]any)
		Expect(request["url"]).To(Equal(
			"https://jobv3-cloud.apigw.te.example/prod/api/v3/get_job_instance_status",
		))
		params := request["params"].(map[string]any)
		Expect(params["bk_biz_id"]).To(Equal(float64(2)))
		Expect(params["job_instance_id"]).To(Equal(float64(100)))
	})

	It("rewrites the paas gateway for legacy templates in dry-run YAML actions", func() {
		restore := config.SetBKTeDomainForTesting("te.example")
		DeferCleanup(restore)
		runtime.Config.BkAPIURLTmpl = "https://{gateway_name}.apigw.te.example"

		action := &system.Action{
			Name:       "get_deployment_result",
			Method:     "GET",
			Path:       "/bkapps/applications/{app_code}/modules/{module}/deployments/{deployment_id}/result/",
			AuthConfig: requiredAuthConfig(),
			Params: []system.Param{
				{Name: "app_code", In: "path", Type: "string", Required: true},
				{Name: "module", In: "path", Type: "string", Required: true},
				{Name: "deployment_id", In: "path", Type: "string", Required: true},
			},
		}
		inputSpec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())

		cmd := buildActionCmd(action)
		Expect(cmd.Flags().Set("app_code", "bk-demo")).To(Succeed())
		Expect(cmd.Flags().Set("module", "default")).To(Succeed())
		Expect(cmd.Flags().Set("deployment_id", "12345")).To(Succeed())

		stdout, err := captureStdout(func() error {
			return system.RunAction(action, inputSpec, "bkpaas3", cmd, runtime, "prod")
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		request := env["request"].(map[string]any)
		Expect(request["url"]).To(Equal(
			"https://paasv3.apigw.te.example/prod/bkapps/applications/bk-demo/modules/default/deployments/12345/result/",
		))
	})

	It("returns a local error when action path placeholders remain unresolved", func() {
		action := &system.Action{
			Name:       "broken_action",
			Method:     "GET",
			Path:       "/api/v1/{defined}/{missing}/",
			AuthConfig: requiredAuthConfig(),
			Params:     []system.Param{{Name: "defined", In: "path", Type: "string", Required: true}},
		}

		cmd := buildActionCmd(action)
		Expect(cmd.Flags().Set("defined", "value")).To(Succeed())
		inputSpec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())

		_, err = captureStdout(func() error {
			return system.RunAction(action, inputSpec, "bk-apigateway", cmd, runtime, "prod")
		})
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("path_error"))
		Expect(cliErr.Message).To(ContainSubstring("unresolved placeholder {missing}"))
	})

	It("returns a local error for missing required params before any request", func() {
		action := &system.Action{
			Name:       "needs_id",
			Method:     "GET",
			Path:       "/api/v1/{id}/",
			AuthConfig: requiredAuthConfig(),
			Params:     []system.Param{{Name: "id", In: "path", Type: "string", Required: true}},
		}

		cmd := buildActionCmd(action)
		inputSpec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())

		stderr, err := captureStderr(func() error {
			return system.RunAction(action, inputSpec, "bk-apigateway", cmd, runtime, "prod")
		})
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("missing_param"))
		Expect(cliErr.Message).To(ContainSubstring("required parameter --id is missing"))
		Expect(stderr).To(ContainSubstring(`"hint": "Usage: bk-cli test-system needs_id --id=VALUE"`))
	})

	It("uses the full nested command path in missing required param hints", func() {
		action := &system.Action{
			Name:       "get_build_list",
			Method:     "GET",
			Path:       "/api/v1/{project_id}/",
			AuthConfig: requiredAuthConfig(),
			Params: []system.Param{
				{Name: "project_id", In: "path", Type: "string", Required: true},
			},
		}

		cmd := buildNestedActionCmd(action)
		inputSpec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())

		stderr, err := captureStderr(func() error {
			return system.RunAction(action, inputSpec, "devops-pipeline", cmd, runtime, "prod")
		})
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("missing_param"))
		Expect(
			stderr,
		).To(
			ContainSubstring(`"hint": "Usage: bk-cli devops pipeline get_build_list --project_id=VALUE"`),
		)
	})

	It("returns a local error when required body is missing", func() {
		action := &system.Action{
			Name:         "create_resource",
			Method:       "POST",
			Path:         "/api/v1/resources/",
			AuthConfig:   requiredAuthConfig(),
			BodyRequired: true,
		}
		inputSpec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())

		cmd := buildActionCmd(action)

		stderr, err := captureStderr(func() error {
			return system.RunAction(action, inputSpec, "bk-apigateway", cmd, runtime, "prod")
		})
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("missing_param"))
		Expect(cliErr.Message).To(ContainSubstring("required parameter --body is missing"))
		Expect(stderr).To(ContainSubstring(`"hint": "Usage: bk-cli test-system create_resource --body=VALUE"`))
	})

	It("returns a local error for invalid body JSON", func() {
		action := &system.Action{
			Name:       "create_resource",
			Method:     "POST",
			Path:       "/api/v1/resources/",
			AuthConfig: requiredAuthConfig(),
		}
		inputSpec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())

		cmd := buildActionCmd(action)
		Expect(cmd.Flags().Set(system.ActionBodyFlagName, `{bad`)).To(Succeed())

		_, err = captureStdout(func() error {
			return system.RunAction(action, inputSpec, "bk-apigateway", cmd, runtime, "prod")
		})
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("request_error"))
		Expect(cliErr.Message).To(ContainSubstring("invalid --body JSON"))
	})

	It("returns a local error for invalid gateway_name path params", func() {
		action := &system.Action{
			Name:       "list_gateway_apis",
			Method:     "GET",
			Path:       "/api/v1/gateways/{gateway_name}/resources/",
			AuthConfig: requiredAuthConfig(),
			Params:     []system.Param{{Name: "gateway_name", In: "path", Type: "string", Required: true}},
		}
		inputSpec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())

		cmd := buildActionCmd(action)
		Expect(cmd.Flags().Set("gateway_name", "bk-iam/extra?x=1")).To(Succeed())

		_, err = captureStdout(func() error {
			return system.RunAction(action, inputSpec, "bk-apigateway", cmd, runtime, "prod")
		})
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("invalid_gateway_name"))
		Expect(cliErr.Message).To(ContainSubstring("gateway_name"))
	})

	It("writes dry-run output to the command writer", func() {
		action := &system.Action{
			Name:       "list_gateway_apis",
			Method:     "GET",
			Path:       "/api/v1/gateways/{gateway_name}/resources/",
			AuthConfig: requiredAuthConfig(),
			Params:     []system.Param{{Name: "gateway_name", In: "path", Type: "string", Required: true}},
		}
		inputSpec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())

		cmd := buildActionCmd(action)
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		Expect(cmd.Flags().Set("gateway_name", "bk-iam")).To(Succeed())

		err = system.RunAction(action, inputSpec, "bk-apigateway", cmd, runtime, "prod")
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal(stdout.Bytes(), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())
		Expect(env["dry_run"]).To(BeTrue())
	})

	It("passes action authConfig through to request execution", func() {
		action := &system.Action{
			Name:       "public_resources",
			Method:     "GET",
			Path:       "/api/v1/public-resources/",
			AuthConfig: &system.AuthConfig{},
		}
		inputSpec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())

		cmd := buildActionCmd(action)
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)

		err = system.RunAction(action, inputSpec, "bk-apigateway", cmd, runtime, "prod")
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal(stdout.Bytes(), &env)).To(Succeed())
		request, ok := env["request"].(map[string]any)
		Expect(ok).To(BeTrue())
		headers, ok := request["headers"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(headers).NotTo(HaveKey("X-Bkapi-Authorization"))
	})

	It("returns a local error for malformed header input", func() {
		action := &system.Action{
			Name:       "list_resources",
			Method:     "GET",
			Path:       "/api/v1/resources/",
			AuthConfig: requiredAuthConfig(),
		}
		inputSpec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())

		cmd := buildActionCmd(action)
		Expect(cmd.Flags().Set(system.ActionHeaderFlagName, "missing-separator")).To(Succeed())

		_, err = captureStdout(func() error {
			return system.RunAction(action, inputSpec, "bk-apigateway", cmd, runtime, "prod")
		})
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("request_error"))
		Expect(cliErr.Message).To(ContainSubstring("invalid --header value"))
	})

	It("uses the action timeout instead of the context timeout when configured", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(150 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		DeferCleanup(server.Close)

		cfg = &config.Config{
			BkAPIURLTmpl: strings.TrimSuffix(server.URL, "/") + "/{gateway_name}",
			Timeout:      50 * time.Millisecond,
		}
		runtime = &system.Runtime{
			Config:     cfg,
			Credential: cred,
			DryRun:     false,
		}

		action := &system.Action{
			Name:        "slow_action",
			Method:      "GET",
			Path:        "/api/v1/resources/",
			AuthConfig:  requiredAuthConfig(),
			Timeout:     "200ms",
			Description: "Slow action",
		}
		inputSpec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())

		cmd := buildActionCmd(action)
		_, err = captureStdout(func() error {
			return system.RunAction(action, inputSpec, "bk-apigateway", cmd, runtime, "prod")
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns a request error for an invalid action timeout", func() {
		action := &system.Action{
			Name:        "bad_timeout",
			Method:      "GET",
			Path:        "/api/v1/resources/",
			AuthConfig:  requiredAuthConfig(),
			Timeout:     "later",
			Description: "Bad timeout",
		}
		inputSpec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())

		cmd := buildActionCmd(action)
		_, err = captureStdout(func() error {
			return system.RunAction(action, inputSpec, "bk-apigateway", cmd, runtime, "prod")
		})
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("request_error"))
		Expect(cliErr.Message).To(ContainSubstring("invalid action timeout"))
	})
})
