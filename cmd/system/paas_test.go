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
	"os"
	"path/filepath"
	"strings"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

var _ = Describe("PaaS plans and build configuration", func() {
	BeforeEach(func() {
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", GinkgoT().TempDir())
	})

	DescribeTable(
		"sends the declared GET route and preserves the response",
		func(name, path, response string, extra []string) {
			var received bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				received = true
				Expect(r.Method).To(Equal(http.MethodGet))
				Expect(
					r.URL.Path,
				).To(
					Equal("/bkpaas3/test/bkapps/applications/bk-demo/modules/api/" + path),
				)
				Expect(r.URL.RawQuery).To(BeEmpty())
				Expect(r.Header.Get("X-Bkapi-Authorization")).NotTo(BeEmpty())
				Expect(r.Header.Get("X-Example")).To(Equal("paas"))
				_, _ = w.Write([]byte(response))
			}))
			DeferCleanup(server.Close)
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			root := &cobra.Command{Use: "bk-cli"}
			Expect(registerSystemSpecs(root, systemCatalog(), testBuildDeps(nil), actionsFS)).To(Succeed())
			args := []string{
				"paas",
				name,
				"--app_code",
				"bk-demo",
				"--module",
				"api",
				"--stage",
				"test",
				"--header",
				"X-Example:paas",
			}
			root.SetArgs(append(args, extra...))
			stdout, err := systemtest.CaptureCommandStdout(root.Execute)
			Expect(err).NotTo(HaveOccurred())
			Expect(received).To(BeTrue())
			var env output.Envelope
			Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
			Expect(env.OK).To(BeTrue())
			Expect(env.Status).To(Equal(http.StatusOK))
			actual, err := json.Marshal(env.Data)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(MatchJSON(response))
		},
		Entry(
			"static plans",
			"list_service_possible_plans",
			"services/svc-uuid/possible_plans",
			`{"has_multiple_plans":true,"static_plans":[{"uuid":"plan-1","name":"shared","description":"shared plan"}],"env_specific_plans":null}`,
			[]string{"--service_id", "svc-uuid"},
		),
		Entry(
			"environment plans",
			"list_service_possible_plans",
			"services/svc-uuid/possible_plans",
			`{"has_multiple_plans":true,"static_plans":null,"env_specific_plans":{"stag":[{"uuid":"plan-stag","name":"small","description":"stag plan"}],"prod":[{"uuid":"plan-prod","name":"large","description":"prod plan"}]}}`,
			[]string{"--service_id", "svc-uuid"},
		),
		Entry(
			"current configuration",
			"get_module_build_config",
			"build_config/",
			`{"build_method":"buildpack","bp_stack_name":"stack-a","buildpacks":[{"id":54,"name":"python"}],"use_bk_ci_pipeline":true}`,
			[]string{},
		),
		Entry(
			"image-specific buildpack IDs",
			"list_module_bp_runtimes",
			"bp_runtimes/",
			`[{"image":"stack-a","slugbuilder":{"display_name":"Ubuntu24"},"buildpacks":[{"id":54,"name":"python","language":"Python","display_name":"blueking/python"}]},{"image":"stack-b","buildpacks":[{"id":82,"name":"python","language":"Python"}]}]`,
			[]string{},
		),
	)

	It("submits the complete build configuration file once and accepts an empty 204 response", func() {
		body := `{"build_method":"buildpack","tag_options":{"prefix":null,"with_version":true,"with_build_time":true,"with_commit_id":false},"bp_stack_name":"stack-a","buildpacks":[{"id":54},{"id":48}],"use_bk_ci_pipeline":true}`
		file := filepath.Join(GinkgoT().TempDir(), "build_config.json")
		Expect(os.WriteFile(file, []byte(body), 0o600)).To(Succeed())
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			requests++
			Expect(r.Method).To(Equal(http.MethodPost))
			Expect(
				r.URL.Path,
			).To(
				Equal("/bkpaas3/prod/bkapps/applications/bk-demo/modules/default/build_config/"),
			)
			Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))
			var payload json.RawMessage
			Expect(json.NewDecoder(r.Body).Decode(&payload)).To(Succeed())
			Expect(payload).To(MatchJSON(body))
			w.WriteHeader(http.StatusNoContent)
		}))
		DeferCleanup(server.Close)
		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		root := &cobra.Command{Use: "bk-cli"}
		Expect(registerSystemSpecs(root, systemCatalog(), testBuildDeps(nil), actionsFS)).To(Succeed())
		root.SetArgs(
			[]string{"paas", "update_module_build_config", "--app_code", "bk-demo", "--body", "@" + file},
		)
		stdout, err := systemtest.CaptureCommandStdout(root.Execute)
		Expect(err).NotTo(HaveOccurred())
		Expect(requests).To(Equal(1))
		var env output.Envelope
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		Expect(env.OK).To(BeTrue())
		Expect(env.Status).To(Equal(http.StatusNoContent))
	})

	DescribeTable(
		"rejects missing inputs before loading credentials",
		func(args []string, message string) {
			root := &cobra.Command{Use: "bk-cli", SilenceUsage: true, SilenceErrors: true}
			Expect(registerSystemSpecs(root, systemCatalog(), testBuildDeps(nil), actionsFS)).To(Succeed())
			root.SetArgs(append([]string{"paas"}, args...))
			err := root.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(message))
		},
		Entry("application", []string{"get_module_build_config"}, `required flag(s) "app_code" not set`),
		Entry(
			"service",
			[]string{"list_service_possible_plans", "--app_code", "bk-demo"},
			`required flag(s) "service_id" not set`,
		),
		Entry(
			"body",
			[]string{"update_module_build_config", "--app_code", "bk-demo"},
			"required parameter --body is missing",
		),
	)

	DescribeTable(
		"explains the upstream plan and replacement contracts in help",
		func(name string, schema bool, fragments []string) {
			root := &cobra.Command{Use: "bk-cli"}
			var out bytes.Buffer
			root.SetOut(&out)
			Expect(registerSystemSpecs(root, systemCatalog(), testBuildDeps(nil), actionsFS)).To(Succeed())
			args := []string{"paas", name, "-h"}
			if schema {
				args = append(args, "--body-schema")
			}
			root.SetArgs(args)
			Expect(root.Execute()).To(Succeed())
			for _, fragment := range fragments {
				Expect(out.String()).To(ContainSubstring(fragment))
			}
			if schema {
				var payload map[string]any
				Expect(
					json.Unmarshal(
						[]byte(strings.TrimPrefix(out.String(), "Request body schema:\n")),
						&payload,
					),
				).To(
					Succeed(),
				)
			}
		},
		Entry(
			"update usage",
			"update_module_build_config",
			false,
			[]string{"整体覆盖", "use_bk_ci_pipeline", "false", "--body @build_config.json", "--body-schema"},
		),
		Entry(
			"update schema",
			"update_module_build_config",
			true,
			[]string{"build_method", "buildpacks", "所选基础镜像", "use_bk_ci_pipeline"},
		),
		Entry(
			"binding schema",
			"bind_service",
			true,
			[]string{"static_plans", "只有一个可选方案时可不传", "env_specific_plans", "env_plan_id_map"},
		),
	)
})
