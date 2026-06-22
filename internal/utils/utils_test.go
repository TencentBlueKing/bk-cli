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

package utils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("utils", func() {
	It("parses non-empty CSV fields", func() {
		Expect(ParseCSVFields(" bk_host_id, ,bk_host_innerip ,")).To(Equal([]string{
			"bk_host_id",
			"bk_host_innerip",
		}))
	})

	It("parses positive CSV ints", func() {
		values, err := ParseCSVInts("1, 2,3", "bk_host_ids", "Use --bk_host_ids 1,2,3")
		Expect(err).NotTo(HaveOccurred())
		Expect(values).To(Equal([]int{1, 2, 3}))
	})

	It("detects IPv4 values", func() {
		Expect(IsIPv4("10.0.0.1")).To(BeTrue())
		Expect(IsIPv4("not-an-ip")).To(BeFalse())
		Expect(IsIPv4("2001:db8::1")).To(BeFalse())
	})

	It("formats Cobra command examples with consistent indentation", func() {
		Expect(FormatCommandExamples(
			"bk-cli foo list",
			"bk-cli foo get --id 1",
		)).To(Equal("  bk-cli foo list\n  bk-cli foo get --id 1"))
	})
})
