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
	"github.com/spf13/cobra"
)

var _ = Describe("systemcmd flag helpers", func() {
	It("registers the standard request flags including body", func() {
		var (
			stage   string
			body    string
			headers []string
		)

		cmd := &cobra.Command{Use: "test"}
		AddCommonRequestFlags(cmd, &stage, &body, &headers)

		Expect(cmd.Flag("stage")).NotTo(BeNil())
		Expect(cmd.Flag("stage").DefValue).To(Equal("prod"))
		Expect(cmd.Flag("body")).NotTo(BeNil())
		Expect(cmd.Flag("body").Usage).To(Equal(
			"[common] Optional; JSON request body; Overrides synthesized body inputs when provided",
		))
		Expect(cmd.Flag("header")).NotTo(BeNil())
		Expect(cmd.Flag("header").Usage).To(Equal(
			"[common] Optional; Additional headers (key:value, repeatable; auth/tenant overrides allowed)",
		))
	})

	It("registers the standard request flags without body", func() {
		var (
			stage   string
			headers []string
		)

		cmd := &cobra.Command{Use: "test"}
		AddCommonRequestFlagsWithoutBody(cmd, &stage, &headers)

		Expect(cmd.Flag("stage")).NotTo(BeNil())
		Expect(cmd.Flag("stage").DefValue).To(Equal("prod"))
		Expect(cmd.Flag("body")).To(BeNil())
		Expect(cmd.Flag("header")).NotTo(BeNil())
	})
})
