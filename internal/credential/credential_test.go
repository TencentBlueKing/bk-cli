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

package credential_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/credential"
)

var _ = Describe("Credential", func() {
	Describe("Validate", func() {
		It("accepts valid app_user with bk_token", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "myapp",
				BkAppSecret: "secret123",
				BkToken:     "token123",
			}
			Expect(cred.Validate()).To(Succeed())
		})

		It("accepts valid app_user with bk_ticket", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "myapp",
				BkAppSecret: "secret123",
				BkTicket:    "ticket123",
			}
			Expect(cred.Validate()).To(Succeed())
		})

		It("accepts valid access_token", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAccessToken,
				AccessToken: "at-12345",
			}
			Expect(cred.Validate()).To(Succeed())
		})

		It("rejects missing bk_app_code", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppSecret: "secret123",
				BkToken:     "token123",
			}
			err := cred.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("bk_app_code"))
		})

		It("rejects missing bk_app_secret", func() {
			cred := &credential.Credential{
				Type:      credential.TypeAppUser,
				BkAppCode: "myapp",
				BkToken:   "token123",
			}
			err := cred.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("bk_app_secret"))
		})

		It("rejects missing both bk_token and bk_ticket", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "myapp",
				BkAppSecret: "secret123",
			}
			err := cred.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("bk_token or bk_ticket"))
		})

		It("rejects missing access_token", func() {
			cred := &credential.Credential{
				Type: credential.TypeAccessToken,
			}
			err := cred.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("access_token"))
		})

		It("rejects unknown type", func() {
			cred := &credential.Credential{
				Type: "magic",
			}
			err := cred.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown credential type"))
		})
	})

	Describe("Marshal/Unmarshal", func() {
		It("round-trips app_user credential", func() {
			original := &credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "myapp",
				BkAppSecret: "secret123",
				BkToken:     "token123",
			}
			data, err := original.Marshal()
			Expect(err).NotTo(HaveOccurred())

			restored, err := credential.Unmarshal(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(restored.Type).To(Equal(original.Type))
			Expect(restored.BkAppCode).To(Equal(original.BkAppCode))
			Expect(restored.BkAppSecret).To(Equal(original.BkAppSecret))
			Expect(restored.BkToken).To(Equal(original.BkToken))
		})

		It("round-trips access_token credential", func() {
			original := &credential.Credential{
				Type:        credential.TypeAccessToken,
				AccessToken: "at-12345",
			}
			data, err := original.Marshal()
			Expect(err).NotTo(HaveOccurred())

			restored, err := credential.Unmarshal(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(restored.Type).To(Equal(original.Type))
			Expect(restored.AccessToken).To(Equal(original.AccessToken))
		})

		It("round-trips app_user with bk_ticket", func() {
			original := &credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "myapp",
				BkAppSecret: "secret123",
				BkTicket:    "ticket456",
			}
			data, err := original.Marshal()
			Expect(err).NotTo(HaveOccurred())

			restored, err := credential.Unmarshal(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(restored.BkTicket).To(Equal(original.BkTicket))
		})
	})

	Describe("MaskedAppCode", func() {
		It("masks middle characters for long codes", func() {
			cred := &credential.Credential{BkAppCode: "abcdefgh"}
			Expect(cred.MaskedAppCode()).To(Equal("ab***gh"))
		})

		It("returns short codes unchanged", func() {
			cred := &credential.Credential{BkAppCode: "ab"}
			Expect(cred.MaskedAppCode()).To(Equal("ab"))

			cred2 := &credential.Credential{BkAppCode: "abcd"}
			Expect(cred2.MaskedAppCode()).To(Equal("abcd"))
		})
	})

	Describe("UserKeyType", func() {
		It("returns bk_token when set", func() {
			cred := &credential.Credential{BkToken: "tok"}
			Expect(cred.UserKeyType()).To(Equal("bk_token"))
		})

		It("returns bk_ticket when set", func() {
			cred := &credential.Credential{BkTicket: "tkt"}
			Expect(cred.UserKeyType()).To(Equal("bk_ticket"))
		})

		It("returns empty string when neither set", func() {
			cred := &credential.Credential{}
			Expect(cred.UserKeyType()).To(BeEmpty())
		})
	})
})
