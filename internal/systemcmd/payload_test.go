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

package systemcmd

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("payload helpers", func() {
	It("marshals payloads to JSON", func() {
		body, err := MarshalJSON(struct {
			Name string `json:"name"`
		}{Name: "demo"})

		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(Equal(`{"name":"demo"}`))
	})

	It("returns a structured error when marshaling fails", func() {
		_, err := MarshalJSON(map[string]any{
			"bad": make(chan int),
		})

		Expect(
			err,
		).To(
			MatchError(
				"request_build_failed: failed to marshal request body: json: unsupported type: chan int",
			),
		)
	})
})
