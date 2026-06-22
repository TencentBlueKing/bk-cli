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

var _ = Describe("cmdb list_resource_pool_hosts", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-cmdb-list-resource-pool-hosts-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("synthesizes request fields and paging", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newListResourcePoolHostsCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("fields", "bk_host_id,bk_host_innerip")).To(Succeed())
		Expect(cmd.Flags().Set("host_ips", "10.0.0.1")).To(Succeed())
		Expect(cmd.Flags().Set("start", "10")).To(Succeed())
		Expect(cmd.Flags().Set("limit", "100")).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		requestBody := env["request"].(map[string]any)["body"].(map[string]any)
		Expect(requestBody["fields"]).To(Equal([]any{"bk_host_id", "bk_host_innerip"}))

		page := requestBody["page"].(map[string]any)
		Expect(page["start"]).To(BeNumerically("==", 10))
		Expect(page["limit"]).To(BeNumerically("==", 100))
		Expect(page["sort"]).To(Equal("bk_host_id"))
		Expect(requestBody).To(HaveKey("host_property_filter"))
	})
})
