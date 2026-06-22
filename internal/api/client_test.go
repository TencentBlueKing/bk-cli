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
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/api"
	"github.com/TencentBlueKing/bk-cli/internal/config"
)

var _ = Describe("NewClient", func() {
	It("uses the provided timeout", func() {
		client := api.NewClient(45 * time.Second)
		Expect(client.HTTPClient.Timeout).To(Equal(45 * time.Second))
	})

	It("falls back to the default timeout when unset", func() {
		client := api.NewClient(0)
		Expect(client.HTTPClient.Timeout).To(Equal(config.DefaultTimeout))
	})

	It("can skip TLS certificate verification when requested", func() {
		client := api.NewClient(45*time.Second, api.WithInsecureSkipVerify(true))

		transport, ok := client.HTTPClient.Transport.(*http.Transport)
		Expect(ok).To(BeTrue())
		Expect(transport.TLSClientConfig).NotTo(BeNil())
		Expect(transport.TLSClientConfig.InsecureSkipVerify).To(BeTrue())
	})
})

var _ = Describe("BuildURL", func() {
	Context("path-based template", func() {
		tmpl := "https://bkapi.example.com/api/{gateway_name}/"
		It("renders gateway and default stage", func() {
			url, err := api.BuildURL(tmpl, "bk-iam", "", "/api/v2/foo/")
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("https://bkapi.example.com/api/bk-iam/prod/api/v2/foo/"))
		})
		It("renders with custom stage", func() {
			url, err := api.BuildURL(tmpl, "bk-iam", "testing", "/api/v2/foo/")
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("https://bkapi.example.com/api/bk-iam/testing/api/v2/foo/"))
		})
	})
	Context("subdomain template", func() {
		tmpl := "https://{gateway_name}.example.com"
		It("renders gateway", func() {
			url, err := api.BuildURL(tmpl, "bk-iam", "prod", "/api/v2/foo/")
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("https://bk-iam.example.com/prod/api/v2/foo/"))
		})
	})
	Context("error cases", func() {
		It("errors on empty template", func() {
			_, err := api.BuildURL("", "gw", "prod", "/path")
			Expect(err).To(HaveOccurred())
		})
		It("errors on empty gateway name", func() {
			_, err := api.BuildURL("https://example.com/{gateway_name}/", "", "prod", "/path")
			Expect(err).To(HaveOccurred())
		})
	})
})
