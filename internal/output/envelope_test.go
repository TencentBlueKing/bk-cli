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

package output_test

import (
	"bytes"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/output"
)

var _ = Describe("Envelope", func() {
	Describe("Success", func() {
		It("creates envelope with ok=true and message", func() {
			env := output.Success("operation completed")
			Expect(env.OK).To(BeTrue())
			Expect(env.Message).To(Equal("operation completed"))
			Expect(env.Error).To(BeNil())
		})
	})

	Describe("SuccessData", func() {
		It("creates envelope with ok=true and data", func() {
			data := map[string]string{"key": "value"}
			env := output.SuccessData(data)
			Expect(env.OK).To(BeTrue())
			Expect(env.Data).To(Equal(data))
		})
	})

	Describe("APIResponse", func() {
		It("sets ok=true for 200 status", func() {
			headers := map[string]string{"Content-Type": "application/json"}
			env := output.APIResponse(200, headers, map[string]string{"result": "ok"})
			Expect(env.OK).To(BeTrue())
			Expect(env.Status).To(Equal(200))
			Expect(env.Headers).To(HaveKeyWithValue("Content-Type", "application/json"))
			Expect(env.Data).NotTo(BeNil())
		})

		It("sets ok=false for 400 status", func() {
			env := output.APIResponse(400, nil, nil)
			Expect(env.OK).To(BeFalse())
			Expect(env.Status).To(Equal(400))
		})

		It("includes headers", func() {
			headers := map[string]string{
				"X-Request-Id": "abc123",
				"Content-Type": "application/json",
			}
			env := output.APIResponse(200, headers, nil)
			Expect(env.Headers).To(HaveLen(2))
			Expect(env.Headers).To(HaveKeyWithValue("X-Request-Id", "abc123"))
		})
	})

	Describe("DryRun", func() {
		It("creates dry-run envelope", func() {
			req := &output.DryRunRequest{
				Method: "GET",
				URL:    "https://example.com/api/test/",
				Headers: map[string]string{
					"Authorization": "Bearer xxx",
				},
			}
			env := output.DryRun(req)
			Expect(env.OK).To(BeTrue())
			Expect(env.DryRun).To(BeTrue())
			Expect(env.Request).NotTo(BeNil())
			Expect(env.Request.Method).To(Equal("GET"))
			Expect(env.Request.URL).To(Equal("https://example.com/api/test/"))
		})
	})

	Describe("Err", func() {
		It("creates error envelope with code/message/hint", func() {
			env := output.Err("AUTH_FAILED", "authentication failed", "Run: bk-cli auth login")
			Expect(env.OK).To(BeFalse())
			Expect(env.Error).NotTo(BeNil())
			Expect(env.Error.Code).To(Equal("AUTH_FAILED"))
			Expect(env.Error.Message).To(Equal("authentication failed"))
			Expect(env.Error.Hint).To(Equal("Run: bk-cli auth login"))
		})
	})

	Describe("WriteJSON", func() {
		It("writes valid JSON to buffer", func() {
			env := output.Success("hello")
			var buf bytes.Buffer
			Expect(env.WriteJSON(&buf)).To(Succeed())

			var parsed map[string]any
			Expect(json.Unmarshal(buf.Bytes(), &parsed)).To(Succeed())
			Expect(parsed["ok"]).To(BeTrue())
			Expect(parsed["message"]).To(Equal("hello"))
		})
	})
})
