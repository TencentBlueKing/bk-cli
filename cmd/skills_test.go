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
	"os"
	"regexp"
	"strings"
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

	It("keeps AIDev skill examples and command sections aligned with registered actions", func() {
		content, err := os.ReadFile("../skills/bk-cli-aidev/SKILL.md")
		Expect(err).NotTo(HaveOccurred())
		root := newRootCmd()
		aidev, remaining, err := root.Find([]string{"aidev"})
		Expect(err).NotTo(HaveOccurred())
		Expect(remaining).To(BeEmpty())
		sections := regexp.MustCompile(
			"(?m)^#### `([a-z][a-z0-9_]*)`$",
		).FindAllStringSubmatch(
			string(content),
			-1,
		)
		documented := make([]string, 0, len(sections))
		for _, section := range sections {
			documented = append(documented, section[1])
		}
		registered := make([]string, 0, len(aidev.Commands()))
		for _, action := range aidev.Commands() {
			registered = append(registered, action.Name())
		}
		Expect(documented).To(ConsistOf(registered))
		for _, match := range regexp.MustCompile(`bk-cli aidev ([a-z][a-z0-9_]*)`).FindAllStringSubmatch(string(content), -1) {
			action, remaining, err := root.Find([]string{"aidev", match[1]})
			Expect(err).NotTo(HaveOccurred(), "skill example: %s", match[0])
			Expect(remaining).To(BeEmpty(), "skill example: %s", match[0])
			Expect(action.Parent()).To(Equal(aidev), "skill example: %s", match[0])
		}
	})

	DescribeTable("keeps AIDev examples actionable in CLI help and the usage skill", func(name string) {
		content, err := os.ReadFile("../skills/bk-cli-aidev/SKILL.md")
		Expect(err).NotTo(HaveOccurred())
		root := newRootCmd()
		bodyPattern := regexp.MustCompile(`--body '([^']+)'`)
		action, _, findErr := root.Find([]string{"aidev", name})
		Expect(findErr).NotTo(HaveOccurred())
		_, section, found := strings.Cut(string(content), "#### `"+name+"`")
		Expect(found).To(BeTrue())
		section, _, _ = strings.Cut(section, "\n#### ")
		for source, example := range map[string]string{"help": action.Example, "skill": section} {
			bodies := bodyPattern.FindAllStringSubmatch(example, -1)
			Expect(bodies).NotTo(BeEmpty(), "%s %s needs a body example", source, name)
			for _, match := range bodies {
				var body map[string]any
				Expect(json.Unmarshal([]byte(match[1]), &body)).To(Succeed())
				Expect(body["space_id"]).To(Equal("demo-space"), "%s %s", source, name)
				switch name {
				case "create_mcp":
					Expect(body["url"]).To(MatchRegexp(`^https?://[^/]+`))
					if body["credential_type"] == "custom" {
						Expect(body["credential_id"]).To(BeNumerically(">", 0))
					}
				case "create_knowledge":
					Expect(body["file_path"]).To(Equal(body["file_name"]))
					Expect(body["url"]).To(BeAssignableToTypeOf(""))
					Expect(body["url"]).NotTo(BeEmpty())
				case "import_knowledge_archive":
					Expect(body["anchor_path"]).To(MatchRegexp(`^/[0-9]+(/.*)?$`))
				case "upsert_collection":
					messages, ok := body["content"].([]any)
					Expect(ok).To(BeTrue())
					Expect(messages).NotTo(BeEmpty())
					Expect(messages[0]).To(HaveKeyWithValue("role", "system"))
					Expect(messages[0]).To(HaveKeyWithValue("content", Not(BeEmpty())))
				case "reclean_knowledge", "trigger_knowledge_update":
					Expect(body["id"]).To(BeNumerically(">", 0))
				case "append_agent_admins":
					Expect(body["admins"]).NotTo(BeEmpty())
				case "get_upload_url":
					Expect(body["module"]).To(Equal("skill"))
				case "create_credential", "update_credential":
					Expect(body["credential_detail"]).To(HaveKeyWithValue("key", Not(BeEmpty())))
					Expect(body["credential_detail"]).To(HaveKeyWithValue("value", Not(BeEmpty())))
				case "batch_delete_credentials":
					Expect(body["credential_ids"]).NotTo(BeEmpty())
				case "batch_create_knowledges", "batch_update_knowledges":
					Expect(body["items"]).NotTo(BeEmpty())
				case "batch_delete_knowledges":
					Expect(body["ids"]).NotTo(BeEmpty())
				case "apply_cross_space_resources":
					Expect(body["resource_ids"]).NotTo(BeEmpty())
				}
			}
		}
	},
		Entry("archive_skill", "archive_skill"),
		Entry("archive_agent", "archive_agent"),
		Entry("delete_knowledgebase", "delete_knowledgebase"),
		Entry("delete_knowledge", "delete_knowledge"),
		Entry("delete_credential", "delete_credential"),
		Entry("create_mcp", "create_mcp"),
		Entry("create_knowledge", "create_knowledge"),
		Entry("import_knowledge_archive", "import_knowledge_archive"),
		Entry("upsert_collection", "upsert_collection"),
		Entry("reclean_knowledge", "reclean_knowledge"),
		Entry("trigger_knowledge_update", "trigger_knowledge_update"),
		Entry("append_agent_admins", "append_agent_admins"),
		Entry("get_upload_url", "get_upload_url"),
		Entry("create_credential", "create_credential"),
		Entry("update_credential", "update_credential"),
		Entry("batch_delete_credentials", "batch_delete_credentials"),
		Entry("batch_create_knowledges", "batch_create_knowledges"),
		Entry("batch_update_knowledges", "batch_update_knowledges"),
		Entry("batch_delete_knowledges", "batch_delete_knowledges"),
		Entry("apply_cross_space_resources", "apply_cross_space_resources"),
	)

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

	It("reads a skill SKILL.md as structured JSON by default", func() {
		cmd := newRootCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"skills", "read", "bk-cli-shared"})

		err := cmd.Execute()
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal(out.Bytes(), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())

		data := env["data"].(map[string]any)
		Expect(data["skill"]).To(Equal("bk-cli-shared"))
		Expect(data["path"]).To(Equal("SKILL.md"))
		Expect(data["content"]).To(Equal(sharedSkill))
	})

	It("reads a reference file under a skill as structured JSON by default", func() {
		cmd := newRootCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"skills", "read", "bk-cli-shared/references/api-debug.md"})

		err := cmd.Execute()
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal(out.Bytes(), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())

		data := env["data"].(map[string]any)
		Expect(data["skill"]).To(Equal("bk-cli-shared"))
		Expect(data["path"]).To(Equal("references/api-debug.md"))
		Expect(data["content"]).To(Equal(apiDebug))
	})

	It("reads a skill SKILL.md as raw markdown with --raw", func() {
		cmd := newRootCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"skills", "read", "bk-cli-shared", "--raw"})

		err := cmd.Execute()
		Expect(err).NotTo(HaveOccurred())
		Expect(out.String()).To(Equal(sharedSkill))
	})

	It("reads a reference file as raw markdown with --raw", func() {
		cmd := newRootCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"skills", "read", "bk-cli-shared/references/api-debug.md", "--raw"})

		err := cmd.Execute()
		Expect(err).NotTo(HaveOccurred())
		Expect(out.String()).To(Equal(apiDebug))
	})

	It("rejects conflicting read output flags", func() {
		cmd := newRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"skills", "read", "bk-cli-shared", "--raw", "--json"})

		err := cmd.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("--raw and --json cannot be used together"))
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
