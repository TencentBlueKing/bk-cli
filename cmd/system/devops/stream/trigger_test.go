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

package stream

import (
	"os"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
)

var _ = Describe("stream trigger", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-devops-stream-trigger-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("passes through the explicit JSON body and escapes path/query values", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newTriggerCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("projectId", "git_12345/alpha")).To(Succeed())
		Expect(cmd.Flags().Set("pipelineId", "pipe?x=1&y=2")).To(Succeed())
		Expect(cmd.Flags().Set(
			"body",
			`{"path":".ci/demo.yml","branch":"main","projectId":"git_12345","customCommitMsg":"manual trigger"}`,
		)).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())

		request := env["request"].(map[string]any)
		Expect(request["url"]).To(Equal(
			"https://bkapi.example.com/api/devops/prod/v4/apigw-user/stream/gitProjects/git_12345%2Falpha/openapi_trigger?pipelineId=pipe%3Fx%3D1%26y%3D2",
		))
		body := request["body"].(map[string]any)
		Expect(body["path"]).To(Equal(".ci/demo.yml"))
		Expect(body["branch"]).To(Equal("main"))
		Expect(body["projectId"]).To(Equal("git_12345"))
		Expect(body["customCommitMsg"]).To(Equal("manual trigger"))
	})
})
