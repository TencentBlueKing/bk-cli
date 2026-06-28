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
	"os"
	"path/filepath"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
)

var _ = Describe("job +run-script", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-job-run-script-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("returns dry-run step previews", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newRunScriptCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("biz", "2")).To(Succeed())
		Expect(cmd.Flags().Set("hosts", "10.0.0.1")).To(Succeed())
		Expect(cmd.Flags().Set("script_content", "echo hello")).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		Expect(env["dry_run"]).To(BeTrue())

		data := env["data"].(map[string]any)
		Expect(data["shortcut"]).To(Equal("job.+run-script"))
		steps := data["steps"].([]any)
		Expect(steps).To(HaveLen(2))
		Expect(steps[0].(map[string]any)["name"]).To(Equal("resolve_hosts"))
		Expect(steps[1].(map[string]any)["name"]).To(Equal("fast_execute_script"))
	})

	It("requires hosts", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newRunScriptCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("biz", "2")).To(Succeed())
		Expect(cmd.Flags().Set("script_content", "echo hello")).To(Succeed())

		err := cmd.RunE(cmd, nil)
		Expect(err).To(MatchError(ContainSubstring("host_ips must include at least one host entry")))
	})

	It("does not expose raw body input", func() {
		cmd := newRunScriptCmd(systemtest.BuildDeps(true))

		Expect(cmd.Flags().Lookup("body")).To(BeNil())
	})

	It("enforces mutual exclusion for script_content and script_file", func() {
		scriptPath := filepath.Join(tmpDir, "script.sh")
		Expect(os.WriteFile(scriptPath, []byte("echo from file"), 0o600)).To(Succeed())

		cmd := newRunScriptCmd(systemtest.BuildDeps(true))
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
