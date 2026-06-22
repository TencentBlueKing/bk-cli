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

package validate_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

var _ = Describe("ValidateGatewayName", func() {
	It("accepts valid gateway names", func() {
		Expect(validate.ValidateGatewayName("bk-iam")).To(Succeed())
		Expect(validate.ValidateGatewayName("bk-demo-2")).To(Succeed())
		Expect(validate.ValidateGatewayName("abc")).To(Succeed())
	})

	It("rejects invalid gateway names", func() {
		Expect(validate.ValidateGatewayName("Bk-iam")).To(MatchError(ContainSubstring("gateway_name")))
		Expect(validate.ValidateGatewayName("1demo")).To(MatchError(ContainSubstring("gateway_name")))
		Expect(validate.ValidateGatewayName("ab")).To(MatchError(ContainSubstring("gateway_name")))
		Expect(validate.ValidateGatewayName("bk_iam")).To(MatchError(ContainSubstring("gateway_name")))
		Expect(validate.ValidateGatewayName("bk-iam/extra")).To(MatchError(ContainSubstring("gateway_name")))
		Expect(validate.ValidateGatewayName("bk-iam?x=1")).To(MatchError(ContainSubstring("gateway_name")))
	})
})

var _ = Describe("ValidateContextName", func() {
	It("accepts safe context names", func() {
		Expect(validate.ValidateContextName("default")).To(Succeed())
		Expect(validate.ValidateContextName("prod-1")).To(Succeed())
		Expect(validate.ValidateContextName("clouds")).To(Succeed())
	})

	It("rejects unsafe context names", func() {
		Expect(validate.ValidateContextName("../x")).To(MatchError(ContainSubstring("context name")))
		Expect(validate.ValidateContextName("a/b")).To(MatchError(ContainSubstring("context name")))
		Expect(validate.ValidateContextName(".")).To(MatchError(ContainSubstring("context name")))
		Expect(validate.ValidateContextName("..")).To(MatchError(ContainSubstring("context name")))
		Expect(validate.ValidateContextName("Prod")).To(MatchError(ContainSubstring("context name")))
		Expect(validate.ValidateContextName("ctx_name")).To(MatchError(ContainSubstring("context name")))
	})
})

var _ = Describe("ValidateHeaderName", func() {
	It("accepts RFC token names", func() {
		Expect(validate.ValidateHeaderName("X-Request-Id")).To(Succeed())
		Expect(validate.ValidateHeaderName("Content-Type")).To(Succeed())
	})

	It("rejects invalid header names", func() {
		Expect(validate.ValidateHeaderName("Bad Header")).To(MatchError(ContainSubstring("header name")))
		Expect(validate.ValidateHeaderName("")).To(MatchError(ContainSubstring("header name")))
		Expect(validate.ValidateHeaderName("X:Bad")).To(MatchError(ContainSubstring("header name")))
	})
})

var _ = Describe("ValidateHeaderValue", func() {
	It("accepts empty and normal values", func() {
		Expect(validate.ValidateHeaderValue("")).To(Succeed())
		Expect(validate.ValidateHeaderValue("trace-123")).To(Succeed())
	})

	It("rejects invalid control characters", func() {
		Expect(validate.ValidateHeaderValue("bad\nvalue")).To(MatchError(ContainSubstring("header value")))
		Expect(validate.ValidateHeaderValue("bad\rvalue")).To(MatchError(ContainSubstring("header value")))
		Expect(validate.ValidateHeaderValue("bad\x00value")).To(MatchError(ContainSubstring("header value")))
	})
})
