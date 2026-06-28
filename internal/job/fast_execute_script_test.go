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

package job

import (
	"encoding/base64"
	"os"
	"path/filepath"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("FastExecuteScript", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-internal-job-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("builds the synthesized request body from flags", func() {
		body, err := BuildFastExecuteScriptBody(FastExecuteScriptInput{
			BizID:          2,
			ScriptContent:  "echo hello",
			ScriptLanguage: "shell",
			TargetServer:   map[string]any{"host_id_list": []int{1}},
			AccountAlias:   "root",
			Timeout:        7200,
		})
		Expect(err).NotTo(HaveOccurred())

		var payload map[string]any
		Expect(json.Unmarshal([]byte(body), &payload)).To(Succeed())
		Expect(payload).To(HaveKeyWithValue("bk_biz_id", float64(2)))
		Expect(payload).To(HaveKeyWithValue("bk_scope_type", "biz"))
		Expect(payload).To(HaveKeyWithValue("bk_scope_id", "2"))
		Expect(payload).To(HaveKeyWithValue("script_language", float64(1)))
		Expect(
			payload,
		).To(
			HaveKeyWithValue("script_content", base64.StdEncoding.EncodeToString([]byte("echo hello"))),
		)
		Expect(payload).To(HaveKeyWithValue("account_alias", "root"))
		Expect(payload).To(HaveKey("target_server"))
	})

	It("reads script content from script file", func() {
		scriptPath := filepath.Join(tmpDir, "script.sh")
		Expect(os.WriteFile(scriptPath, []byte("echo from file"), 0o600)).To(Succeed())

		body, err := BuildFastExecuteScriptBody(FastExecuteScriptInput{
			BizID:          2,
			ScriptFile:     scriptPath,
			ScriptLanguage: "shell",
			TargetServer:   map[string]any{"host_id_list": []int{1}},
			Timeout:        7200,
		})
		Expect(err).NotTo(HaveOccurred())

		var payload map[string]any
		Expect(json.Unmarshal([]byte(body), &payload)).To(Succeed())
		Expect(
			payload,
		).To(
			HaveKeyWithValue("script_content", base64.StdEncoding.EncodeToString([]byte("echo from file"))),
		)
	})

	It("uses body override without synthesizing", func() {
		body, err := BuildFastExecuteScriptBody(FastExecuteScriptInput{
			BodyOverride: `{"bk_biz_id":2}`,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(Equal(`{"bk_biz_id":2}`))
	})

	It("rejects missing script content and script file", func() {
		_, err := BuildFastExecuteScriptBody(FastExecuteScriptInput{
			BizID:          2,
			ScriptLanguage: "shell",
			TargetServer:   map[string]any{"host_id_list": []int{1}},
			Timeout:        7200,
		})
		Expect(err).To(MatchError(ContainSubstring("one of script_content or script_file is required")))
	})

	It("rejects invalid script languages", func() {
		_, err := BuildFastExecuteScriptBody(FastExecuteScriptInput{
			BizID:          2,
			ScriptContent:  "echo hello",
			ScriptLanguage: "ruby",
			TargetServer:   map[string]any{"host_id_list": []int{1}},
			Timeout:        7200,
		})
		Expect(err).To(MatchError(ContainSubstring("script_language must be one of")))
	})

	It("builds the request spec", func() {
		spec, err := BuildFastExecuteScriptRequest(FastExecuteScriptInput{
			BizID:          2,
			ScriptContent:  "echo hello",
			ScriptLanguage: "shell",
			TargetServer:   map[string]any{"host_id_list": []int{1}},
			Stage:          "prod",
			Headers:        []string{"X-Test:true"},
			Timeout:        7200,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(spec.GatewayName).To(Equal("bk-job"))
		Expect(spec.Method).To(Equal("POST"))
		Expect(spec.Path).To(Equal("/api/v3/fast_execute_script"))
		Expect(spec.Stage).To(Equal("prod"))
		Expect(spec.Headers).To(Equal([]string{"X-Test:true"}))
		Expect(spec.AuthConfig).NotTo(BeNil())
	})
})
