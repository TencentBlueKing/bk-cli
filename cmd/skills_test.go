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
	"testing/fstest"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("skills command", func() {
	const sharedSkill = `---
name: bk-cli-shared
description: 共享规则
---

# bk-cli shared

通用使用规则。
`

	const apiDebug = `# API debug

网关错误排查。
`

	BeforeEach(func() {
		SetSkillsFS(fstest.MapFS{
			"skills/bk-cli-shared/SKILL.md": {
				Data: []byte(sharedSkill),
			},
			"skills/bk-cli-shared/references/api-debug.md": {
				Data: []byte(apiDebug),
			},
			"skills/bk-cli-api/SKILL.md": {
				Data: []byte("---\nname: bk-cli-api\ndescription: 原始 API 调用\n---\n\n# bk-cli api\n"),
			},
			"skills/bk-cli-api/scripts/ignored.sh": {
				Data: []byte("echo ignored"),
			},
		})
	})

	AfterEach(func() {
		SetSkillsFS(nil)
	})

	It("lists embedded skills as structured JSON without machine-only files", func() {
		cmd := newRootCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(out)
		cmd.SetArgs([]string{"skills", "list"})

		err := cmd.Execute()
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal(out.Bytes(), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())

		data := env["data"].(map[string]any)
		Expect(data["count"]).To(Equal(float64(2)))
		Expect(data["skills"]).To(ConsistOf(
			HaveKeyWithValue("name", "bk-cli-api"),
			HaveKeyWithValue("name", "bk-cli-shared"),
		))
		Expect(out.String()).NotTo(ContainSubstring("ignored.sh"))
	})

	It("reads a skill SKILL.md as raw markdown by default", func() {
		cmd := newRootCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"skills", "read", "bk-cli-shared"})

		err := cmd.Execute()
		Expect(err).NotTo(HaveOccurred())
		Expect(out.String()).To(Equal(sharedSkill))
	})

	It("reads a reference file under a skill", func() {
		cmd := newRootCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"skills", "read", "bk-cli-shared/references/api-debug.md"})

		err := cmd.Execute()
		Expect(err).NotTo(HaveOccurred())
		Expect(out.String()).To(Equal(apiDebug))
	})

	It("rejects path traversal when reading skill content", func() {
		cmd := newRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"skills", "read", "../AGENTS.md"})

		err := cmd.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid skill path"))
	})
})
