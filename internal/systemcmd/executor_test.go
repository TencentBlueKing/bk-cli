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

	"github.com/TencentBlueKing/bk-cli/internal/output"
)

var _ = Describe("systemcmd executor helpers", func() {
	It("rejects empty response envelopes", func() {
		err := EnsureEnvelope("demo_action", nil)

		Expect(err).To(MatchError("demo_action received an empty response envelope"))
	})

	It("accepts non-empty response envelopes", func() {
		err := EnsureEnvelope("demo_action", &output.Envelope{})

		Expect(err).NotTo(HaveOccurred())
	})
})
