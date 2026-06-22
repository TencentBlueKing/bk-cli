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

package sops

import (
	"os"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
)

var _ = Describe("sops create_task", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-sops-create-task-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("encodes constants as a nested JSON object", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newCreateTaskCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("bk_biz_id", "2")).To(Succeed())
		Expect(cmd.Flags().Set("template_id", "100")).To(Succeed())
		Expect(cmd.Flags().Set("name", "deploy")).To(Succeed())
		Expect(cmd.Flags().Set("constants", `{"${key}":"value"}`)).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())

		request := env["request"].(map[string]any)
		body := request["body"].(map[string]any)
		Expect(body).To(HaveKeyWithValue("name", "deploy"))
		Expect(body).To(HaveKeyWithValue("flow_type", "common"))
		Expect(body).To(HaveKeyWithValue("constants", map[string]any{
			"${key}": "value",
		}))
	})

	It("rejects constants that are not a JSON object", func() {
		_, err := buildCreateTaskBody("", "deploy", `["bad"]`)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("constants"))
	})
})
