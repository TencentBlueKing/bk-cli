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

package testutil

import (
	"errors"
	"fmt"
	"testing/fstest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("shared system test helpers", func() {
	It("builds shared deps for dry-run tests", func() {
		deps := BuildDeps(true)

		Expect(deps.GetContext()).To(BeEmpty())
		Expect(deps.IsDryRun()).To(BeTrue())
		Expect(deps.IsVerbose()).To(BeFalse())
	})

	It("captures stdout while preserving the returned error", func() {
		stdout, err := CaptureCommandStdout(func() error {
			fmt.Print("hello")
			return errors.New("boom")
		})

		Expect(stdout).To(Equal("hello"))
		Expect(err).To(MatchError("boom"))
	})

	It("builds a YAML action command for tests", func() {
		yamlFS := fstest.MapFS{
			"actions.yaml": {
				Data: []byte(`
name: demo
gateway_name: bk-demo
description: Demo service
actions:
  - name: list_items
    description: List items
    method: GET
    path: /api/v1/items/
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
    params:
      - name: keyword
        in: query
        type: string
`),
			},
		}

		cmd, err := BuildYAMLActionCmd(yamlFS, "actions.yaml", "list_items", BuildDeps(true))
		Expect(err).NotTo(HaveOccurred())
		Expect(cmd.Name()).To(Equal("list_items"))
		Expect(cmd.Flag("stage")).NotTo(BeNil())
		Expect(cmd.Flag("body")).NotTo(BeNil())
		Expect(cmd.Flag("header")).NotTo(BeNil())
		Expect(cmd.Flag("keyword")).NotTo(BeNil())
	})
})
