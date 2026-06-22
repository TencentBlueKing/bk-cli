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

package system_test

import (
	"testing/fstest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/system"
)

var _ = Describe("additional system coverage", func() {
	It("uses permissive auth defaults when auth config is nil", func() {
		var authConfig *system.AuthConfig
		Expect(authConfig.RequiresAuth()).To(BeTrue())
		Expect(authConfig.RequiresAppVerification()).To(BeTrue())
		Expect(authConfig.RequiresUserVerification()).To(BeTrue())
	})

	It("reports no auth when both verification modes are disabled", func() {
		authConfig := &system.AuthConfig{}
		Expect(authConfig.RequiresAuth()).To(BeFalse())
		Expect(authConfig.RequiresAppVerification()).To(BeFalse())
		Expect(authConfig.RequiresUserVerification()).To(BeFalse())
	})

	It("loads all YAML systems from an fs and skips non-YAML files", func() {
		yamlFS := fstest.MapFS{
			"actions/demo.yaml": {Data: []byte(`
name: demo
gateway_name: bk-demo
actions:
  - name: list_items
    method: GET
    path: "/api/v1/items/"
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
`)},
			"actions/notes.txt": {Data: []byte("ignore me")},
			"actions/nested":    {Mode: 0o755 | 1<<31},
		}

		systems, err := system.LoadAllFromFS(yamlFS, "actions")
		Expect(err).NotTo(HaveOccurred())
		Expect(systems).To(HaveLen(1))
		Expect(systems[0].Name).To(Equal("demo"))
	})

	It("returns a directory read error when loading all systems", func() {
		_, err := system.LoadAllFromFS(fstest.MapFS{}, "missing")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to read embedded directory"))
	})
})
