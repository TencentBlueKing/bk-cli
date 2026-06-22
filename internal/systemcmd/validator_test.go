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

var _ = Describe("systemcmd validator helpers", func() {
	It("validates positive int flags", func() {
		Expect(ValidatePositiveIntFlag("limit", 1)).To(Succeed())
		Expect(ValidatePositiveIntFlag("limit", 0)).To(MatchError(
			"invalid_argument: limit must be greater than 0",
		))
	})

	It("validates changed positive int flags only when provided", func() {
		Expect(ValidatePositiveIntFlagIfChanged("bk_biz_id", 0, false)).To(Succeed())
		Expect(ValidatePositiveIntFlagIfChanged("bk_biz_id", 0, true)).To(MatchError(
			"invalid_argument: bk_biz_id must be greater than 0",
		))
	})

	It("validates non-negative int flags", func() {
		Expect(ValidateNonNegativeIntFlag("start", 0)).To(Succeed())
		Expect(ValidateNonNegativeIntFlag("start", -1)).To(MatchError(
			"invalid_argument: start must be greater than or equal to 0",
		))
	})

	It("validates non-empty string flags", func() {
		Expect(ValidateNonEmptyStringFlag("bk_module_name", "module")).To(Succeed())
		Expect(ValidateNonEmptyStringFlag("bk_module_name", "   ")).To(MatchError(
			"invalid_argument: bk_module_name cannot be empty",
		))
	})
})
