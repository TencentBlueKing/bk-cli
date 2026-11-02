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

package monitor

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

var _ = Describe("log row filters", func() {
	var dir string
	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", dir)
		GinkgoT().Setenv("BK_CLI_MONITOR_PROFILE_FILE", "")
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
	})

	It("keeps table routing separate from row filters and parenthesizes an OR query", func() {
		cmd := toolCommand("search_logs", true)
		setFlags(
			cmd,
			"bk_biz_id",
			"100100",
			"scene",
			"k8s",
			"where",
			"cluster_id=test-cluster",
			"filter",
			"__ext.io_kubernetes_pod_namespace=ns-a",
			"query_string",
			"ERROR OR WARN",
			"since",
			"15m",
		)
		args := dryRunArgs(cmd)
		Expect(args["table_id_conditions"].([]any)[0]).To(HaveLen(2))
		Expect(args["query_string"]).To(Equal(`(ERROR OR WARN) AND (__ext.io_kubernetes_pod_namespace:"ns-a")`))
		Expect(args).NotTo(HaveKey("filters"))
		Expect(args).NotTo(HaveKey("filter"))
	})

	It("escapes literal quotes and backslashes without changing index-set mode", func() {
		cmd := toolCommand("search_logs", true)
		setFlags(cmd, "bk_biz_id", "100100", "index_set_id", "101003", "filter", `field=a"b\c`, "since", "15m")
		args := dryRunArgs(cmd)
		Expect(args["target_type"]).To(Equal("index_set"))
		Expect(args["query_string"]).To(Equal(`field:"a\"b\\c"`))
	})

	It("keeps Unicode and HTML punctuation literal rather than JSON-escaping the query language", func() {
		cmd := toolCommand("search_logs", true)
		setFlags(
			cmd,
			"bk_biz_id",
			"100100",
			"index_set_id",
			"101003",
			"filter",
			"component=测试<a&b>",
			"since",
			"15m",
		)
		Expect(dryRunArgs(cmd)["query_string"]).To(Equal(`component:"测试<a&b>"`))
	})

	It("overrides profile filters per key while retaining the other defaults", func() {
		file := filepath.Join(dir, "profiles.yaml")
		Expect(os.WriteFile(file, []byte(`version: 1
profiles:
  app:
    context: default
    bk_biz_id: "100100"
    log:
      scene: k8s
      where: {cluster_id: test-cluster}
      filters: {namespace: ns-a, component: api}
`), 0o600)).To(Succeed())
		cmd := toolCommand("search_logs", true)
		setFlags(
			cmd,
			"profile",
			"app",
			"profile_file",
			file,
			"filter",
			"namespace=ns-b",
			"query_string",
			"*",
			"since",
			"15m",
		)
		env, err := runProfileCommand(cmd)
		Expect(err).NotTo(HaveOccurred())
		args := previewArgs(env)
		Expect(args["query_string"]).To(Equal(`component:"api" AND namespace:"ns-b"`))
		Expect(args["table_id_conditions"].([]any)[0]).To(HaveLen(2))
		profile := env["profile"].(map[string]any)
		Expect(profile["applied_fields"]).To(ContainElement("log.filters"))
		Expect(
			profile["resolved_parameters"].(map[string]any)["log_filters"],
		).To(
			HaveKeyWithValue("namespace", "ns-b"),
		)
	})

	DescribeTable("rejects invalid filters locally", func(value string) {
		cmd := toolCommand("search_logs", true)
		setFlags(cmd, "bk_biz_id", "100100", "index_set_id", "101003", "filter", value, "since", "15m")
		_, err := runProfileCommand(cmd)
		Expect(err.(*output.CLIError).Code).To(Equal("invalid_log_filter"))
	},
		Entry("missing value", "namespace="),
		Entry("missing equals", "namespace"),
		Entry("expression in field", "ns) OR *=all"),
		Entry("newline", "namespace=ns-a\nOR *"),
	)
})
