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

package cmdb

import (
	"os"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
)

var _ = Describe("cmdb search_module", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-cmdb-search-module-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("synthesizes the request body from flags", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newSearchModuleCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("bk_biz_id", "2")).To(Succeed())
		Expect(cmd.Flags().Set("bk_set_id", "10")).To(Succeed())
		Expect(cmd.Flags().Set("bk_module_name", "idle")).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		request := env["request"].(map[string]any)
		Expect(request["url"]).To(Equal(
			"https://bkapi.example.com/api/bk-cmdb/prod/api/v3/open/module/search/0/2/10",
		))

		body := request["body"].(map[string]any)
		Expect(body["fields"]).To(Equal([]any{"bk_module_id", "bk_module_name"}))
		Expect(body["condition"]).To(Equal(map[string]any{"bk_module_name": "idle"}))
		page := body["page"].(map[string]any)
		Expect(page["start"]).To(BeNumerically("==", 0))
		Expect(page["limit"]).To(BeNumerically("==", 500))
	})
})
