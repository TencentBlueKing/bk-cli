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

package api_test

import (
	json "github.com/goccy/go-json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/api"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
)

var _ = Describe("BuildAuthHeader", func() {
	parseHeader := func(header string) map[string]string {
		var m map[string]string
		Expect(json.Unmarshal([]byte(header), &m)).To(Succeed())
		return m
	}

	Context("app_user with bk_token", func() {
		It("contains bk_app_code, bk_app_secret, and bk_token", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "mycode",
				BkAppSecret: "mysecret",
				BkToken:     "mytoken",
			}
			header, err := api.BuildAuthHeader(cred, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(header).NotTo(BeEmpty())

			var m map[string]string
			Expect(json.Unmarshal([]byte(header), &m)).To(Succeed())
			Expect(m).To(HaveKeyWithValue("bk_app_code", "mycode"))
			Expect(m).To(HaveKeyWithValue("bk_app_secret", "mysecret"))
			Expect(m).To(HaveKeyWithValue("bk_token", "mytoken"))
			Expect(m).NotTo(HaveKey("bk_ticket"))
			Expect(m).NotTo(HaveKey("access_token"))
		})
	})

	Context("app_user with bk_ticket", func() {
		It("contains bk_app_code, bk_app_secret, and bk_ticket", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "mycode",
				BkAppSecret: "mysecret",
				BkTicket:    "myticket",
			}
			header, err := api.BuildAuthHeader(cred, nil)
			Expect(err).NotTo(HaveOccurred())

			var m map[string]string
			Expect(json.Unmarshal([]byte(header), &m)).To(Succeed())
			Expect(m).To(HaveKeyWithValue("bk_app_code", "mycode"))
			Expect(m).To(HaveKeyWithValue("bk_app_secret", "mysecret"))
			Expect(m).To(HaveKeyWithValue("bk_ticket", "myticket"))
			Expect(m).NotTo(HaveKey("bk_token"))
		})
	})

	Context("app_user app-only (no token/ticket)", func() {
		It("contains only bk_app_code and bk_app_secret", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "mycode",
				BkAppSecret: "mysecret",
			}
			header, err := api.BuildAuthHeader(cred, nil)
			Expect(err).NotTo(HaveOccurred())

			var m map[string]string
			Expect(json.Unmarshal([]byte(header), &m)).To(Succeed())
			Expect(m).To(HaveKeyWithValue("bk_app_code", "mycode"))
			Expect(m).To(HaveKeyWithValue("bk_app_secret", "mysecret"))
			Expect(m).NotTo(HaveKey("bk_token"))
			Expect(m).NotTo(HaveKey("bk_ticket"))
		})
	})

	Context("access_token", func() {
		It("contains only access_token", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAccessToken,
				AccessToken: "tok123",
			}
			header, err := api.BuildAuthHeader(cred, nil)
			Expect(err).NotTo(HaveOccurred())

			var m map[string]string
			Expect(json.Unmarshal([]byte(header), &m)).To(Succeed())
			Expect(m).To(HaveKeyWithValue("access_token", "tok123"))
			Expect(m).NotTo(HaveKey("bk_app_code"))
		})
	})

	Context("nil credential", func() {
		It("returns empty string without error", func() {
			header, err := api.BuildAuthHeader(nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(header).To(BeEmpty())
		})
	})

	Context("auth config", func() {
		It("includes app and user fields when both verifications are required", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "mycode",
				BkAppSecret: "mysecret",
				BkToken:     "mytoken",
			}

			header, err := api.BuildAuthHeader(cred, &api.AuthRequirements{
				AppVerifiedRequired:  true,
				UserVerifiedRequired: true,
			})
			Expect(err).NotTo(HaveOccurred())

			m := parseHeader(header)
			Expect(m).To(HaveKeyWithValue("bk_app_code", "mycode"))
			Expect(m).To(HaveKeyWithValue("bk_app_secret", "mysecret"))
			Expect(m).To(HaveKeyWithValue("bk_token", "mytoken"))
		})

		It("includes only app fields when only app verification is required", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "mycode",
				BkAppSecret: "mysecret",
				BkToken:     "mytoken",
			}

			header, err := api.BuildAuthHeader(cred, &api.AuthRequirements{
				AppVerifiedRequired: true,
			})
			Expect(err).NotTo(HaveOccurred())

			m := parseHeader(header)
			Expect(m).To(HaveKeyWithValue("bk_app_code", "mycode"))
			Expect(m).To(HaveKeyWithValue("bk_app_secret", "mysecret"))
			Expect(m).NotTo(HaveKey("bk_token"))
			Expect(m).NotTo(HaveKey("bk_ticket"))
		})

		It("includes only user fields when only user verification is required", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "mycode",
				BkAppSecret: "mysecret",
				BkTicket:    "myticket",
			}

			header, err := api.BuildAuthHeader(cred, &api.AuthRequirements{
				UserVerifiedRequired: true,
			})
			Expect(err).NotTo(HaveOccurred())

			m := parseHeader(header)
			Expect(m).To(HaveKeyWithValue("bk_ticket", "myticket"))
			Expect(m).NotTo(HaveKey("bk_app_code"))
			Expect(m).NotTo(HaveKey("bk_app_secret"))
		})

		It("uses access_token whenever configured auth requires identity", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAccessToken,
				AccessToken: "tok123",
			}

			for _, authConfig := range []api.AuthRequirements{
				{AppVerifiedRequired: true},
				{UserVerifiedRequired: true},
				{AppVerifiedRequired: true, UserVerifiedRequired: true},
			} {
				header, err := api.BuildAuthHeader(cred, &authConfig)
				Expect(err).NotTo(HaveOccurred())

				m := parseHeader(header)
				Expect(m).To(HaveKeyWithValue("access_token", "tok123"))
				Expect(m).NotTo(HaveKey("bk_app_code"))
				Expect(m).NotTo(HaveKey("bk_token"))
			}
		})

		It("returns no auth header when no verification is required", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "mycode",
				BkAppSecret: "mysecret",
				BkToken:     "mytoken",
			}

			header, err := api.BuildAuthHeader(cred, &api.AuthRequirements{})
			Expect(err).NotTo(HaveOccurred())
			Expect(header).To(BeEmpty())
		})

		It("treats auth requirements as identity-only configuration", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "mycode",
				BkAppSecret: "mysecret",
				BkToken:     "mytoken",
			}

			header, err := api.BuildAuthHeader(cred, &api.AuthRequirements{})
			Expect(err).NotTo(HaveOccurred())
			Expect(header).To(BeEmpty())
		})
	})
})
