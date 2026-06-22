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

package job

import (
	"encoding/base64"
	"os"
	"path/filepath"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
	"github.com/TencentBlueKing/bk-cli/internal/config"
)

var _ = Describe("job fast_execute_script", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-job-fast-execute-script-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("synthesizes target_server into the outgoing request body", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newFastExecuteScriptCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("bk_biz_id", "2")).To(Succeed())
		Expect(cmd.Flags().Set("script_content", "echo hello")).To(Succeed())
		Expect(cmd.Flags().Set("script_language", "shell")).To(Succeed())
		Expect(cmd.Flags().Set("account_alias", "root")).To(Succeed())
		Expect(cmd.Flags().Set("target_server", `{"host_id_list":[1]}`)).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())

		request := env["request"].(map[string]any)
		body := request["body"].(map[string]any)
		Expect(body).To(HaveKeyWithValue("bk_biz_id", float64(2)))
		Expect(body).To(HaveKeyWithValue("bk_scope_type", "biz"))
		Expect(body).To(HaveKeyWithValue("bk_scope_id", "2"))
		Expect(body).To(HaveKeyWithValue(
			"script_content",
			base64.StdEncoding.EncodeToString([]byte("echo hello")),
		))
		Expect(body).To(HaveKeyWithValue("account_alias", "root"))
		Expect(body).To(HaveKeyWithValue("target_server", map[string]any{
			"host_id_list": []any{float64(1)},
		}))
	})

	It("requires target_server when synthesizing from flags", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newFastExecuteScriptCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("bk_biz_id", "2")).To(Succeed())
		Expect(cmd.Flags().Set("script_content", "echo hello")).To(Succeed())

		err := cmd.RunE(cmd, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("target_server"))
	})

	It("reads script content from script_file when synthesizing the request body", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		scriptPath := filepath.Join(tmpDir, "script.sh")
		Expect(os.WriteFile(scriptPath, []byte("echo from file"), 0o600)).To(Succeed())

		cmd := newFastExecuteScriptCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("bk_biz_id", "2")).To(Succeed())
		Expect(cmd.Flags().Set("script_file", scriptPath)).To(Succeed())
		Expect(cmd.Flags().Set("script_language", "shell")).To(Succeed())
		Expect(cmd.Flags().Set("target_server", `{"host_id_list":[1]}`)).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())

		request := env["request"].(map[string]any)
		body := request["body"].(map[string]any)
		Expect(body).To(HaveKeyWithValue(
			"script_content",
			base64.StdEncoding.EncodeToString([]byte("echo from file")),
		))
	})

	It("uses jobv3-cloud when BK_TE_DOMAIN matches the legacy template", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		restore := config.SetBKTeDomainForTesting("te.example")
		DeferCleanup(restore)

		cfg, err := config.Load(config.ConfigPath("default"))
		Expect(err).NotTo(HaveOccurred())
		cfg.BkAPIURLTmpl = "https://{gateway_name}.apigw.te.example"
		Expect(cfg.Save(config.ConfigPath("default"))).To(Succeed())

		cmd := newFastExecuteScriptCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("bk_biz_id", "2")).To(Succeed())
		Expect(cmd.Flags().Set("script_content", "echo hello")).To(Succeed())
		Expect(cmd.Flags().Set("script_language", "shell")).To(Succeed())
		Expect(cmd.Flags().Set("target_server", `{"host_id_list":[1]}`)).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		request := env["request"].(map[string]any)
		Expect(request["url"]).To(Equal(
			"https://jobv3-cloud.apigw.te.example/prod/api/v3/fast_execute_script",
		))
	})

	It("requires one of script_content or script_file when synthesizing from flags", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newFastExecuteScriptCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("bk_biz_id", "2")).To(Succeed())
		Expect(cmd.Flags().Set("target_server", `{"host_id_list":[1]}`)).To(Succeed())

		err := cmd.RunE(cmd, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("one of script_content or script_file is required"))
	})

	It("enforces mutual exclusion for script_content and script_file", func() {
		scriptPath := filepath.Join(tmpDir, "script.sh")
		Expect(os.WriteFile(scriptPath, []byte("echo from file"), 0o600)).To(Succeed())

		cmd := newFastExecuteScriptCmd(systemtest.BuildDeps(true))
		cmd.SetArgs([]string{
			"--script_content", "echo hello",
			"--script_file", scriptPath,
		})
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true

		err := cmd.Execute()
		Expect(err).To(MatchError(ContainSubstring(
			"if any flags in the group [script_content script_file] are set none of the others can be",
		)))
	})
})
