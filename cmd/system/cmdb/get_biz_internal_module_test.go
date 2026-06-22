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
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
)

var _ = Describe("cmdb get_biz_internal_module", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-cmdb-yaml-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("documents supplier_account with the default 0 example", func() {
		sys, err := syslib.LoadSystemFromFS(os.DirFS("."), "actions.yaml")
		Expect(err).NotTo(HaveOccurred())

		var action *syslib.Action
		for i := range sys.Actions {
			if sys.Actions[i].Name == "get_biz_internal_module" {
				action = &sys.Actions[i]
				break
			}
		}

		Expect(action).NotTo(BeNil())
		Expect(action.Examples).To(ContainElement(
			"bk-cli cmdb get_biz_internal_module --supplier_account 0 --bk_biz_id 2",
		))
	})

	It("registers the YAML-backed action flags", func() {
		cmd, err := systemtest.BuildYAMLActionCmd(
			os.DirFS("."),
			"actions.yaml",
			"get_biz_internal_module",
			systemtest.BuildDeps(true),
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(cmd.Flag("bk_biz_id")).NotTo(BeNil())
		Expect(cmd.Flag("supplier_account")).NotTo(BeNil())
		Expect(cmd.Flag("stage")).NotTo(BeNil())
		Expect(cmd.Flag("body")).NotTo(BeNil())
		Expect(cmd.Flag("header")).NotTo(BeNil())
	})

	It("shows the YAML dry-run request", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd, err := systemtest.BuildYAMLActionCmd(
			os.DirFS("."),
			"actions.yaml",
			"get_biz_internal_module",
			systemtest.BuildDeps(true),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(cmd.Flags().Set("bk_biz_id", "2")).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())
		Expect(env["dry_run"]).To(BeTrue())

		request := env["request"].(map[string]any)
		Expect(request["method"]).To(Equal("GET"))
		Expect(request["url"]).To(Equal(
			"https://bkapi.example.com/api/bk-cmdb/prod/api/v3/open/topo/internal/0/2",
		))
		Expect(request).NotTo(HaveKey("body"))
	})
})
