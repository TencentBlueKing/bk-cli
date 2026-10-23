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

package cmd

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MCP root integration", func() {
	It("rejects an explicit context before or after the MCP command", func() {
		for _, args := range [][]string{{"--context=missing", "mcp", "--help"}, {"mcp", "--context=missing", "--help"}} {
			root := newRootCmd()
			var out, stderr bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&stderr)
			err := executeRoot(root, args)
			Expect(err).To(HaveOccurred())
			Expect(stderr.String()).To(ContainSubstring("unsupported_option"))
			Expect(out.String()).To(BeEmpty())
		}
	})
	It("does not apply system bool normalization to dynamic MCP parameters", func() {
		root := newRootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		Expect(executeRoot(root, []string{"mcp", "--help"})).To(Succeed())
		Expect(out.String()).To(ContainSubstring("server"))
	})
})
