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
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/api"
)

var _ = Describe("Request.Build", func() {
	It("sets the default bk-cli user agent", func() {
		r := &api.Request{
			Method: "GET",
			URL:    "https://example.com/api/v1/test",
		}
		req, err := r.Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(req.Header.Get("User-Agent")).To(Equal("bk-cli/dev"))
	})

	It("uses the configured bk-cli version in the user agent", func() {
		api.SetUserAgentVersion("v1.2.3")
		DeferCleanup(api.SetUserAgentVersion, "dev")

		r := &api.Request{
			Method: "GET",
			URL:    "https://example.com/api/v1/test",
		}
		req, err := r.Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(req.Header.Get("User-Agent")).To(Equal("bk-cli/v1.2.3"))
	})

	It("GET request has no body", func() {
		r := &api.Request{
			Method: "GET",
			URL:    "https://example.com/api/v1/test",
		}
		req, err := r.Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(req.Body).To(BeNil())
		Expect(req.Header.Get("Content-Type")).To(BeEmpty())
	})

	It("POST with body sets Content-Type application/json", func() {
		r := &api.Request{
			Method:   "POST",
			URL:      "https://example.com/api/v1/test",
			BodyJSON: `{"key":"value"}`,
		}
		req, err := r.Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(req.Header.Get("Content-Type")).To(Equal("application/json"))
		body, _ := io.ReadAll(req.Body)
		Expect(string(body)).To(Equal(`{"key":"value"}`))
	})

	It("sets auth header correctly", func() {
		r := &api.Request{
			Method:     "GET",
			URL:        "https://example.com/api",
			AuthHeader: `{"bk_app_code":"code"}`,
		}
		req, err := r.Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(req.Header.Get("X-Bkapi-Authorization")).To(Equal(`{"bk_app_code":"code"}`))
	})

	It("sets tenant ID header", func() {
		r := &api.Request{
			Method:   "GET",
			URL:      "https://example.com/api",
			TenantID: "tenant-abc",
		}
		req, err := r.Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(req.Header.Get("X-Bk-Tenant-Id")).To(Equal("tenant-abc"))
	})

	It("adds custom headers", func() {
		r := &api.Request{
			Method: "GET",
			URL:    "https://example.com/api",
			Headers: map[string]string{
				"X-Custom": "hello",
			},
		}
		req, err := r.Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(req.Header.Get("X-Custom")).To(Equal("hello"))
	})

	It("lets custom auth and tenant headers override generated values", func() {
		r := &api.Request{
			Method:     "GET",
			URL:        "https://example.com/api",
			AuthHeader: `{"access_token":"generated"}`,
			TenantID:   "generated-tenant",
			Headers: map[string]string{
				"X-Bkapi-Authorization": "custom-auth",
				"X-Bk-Tenant-Id":        "custom-tenant",
			},
		}
		req, err := r.Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(req.Header.Get("X-Bkapi-Authorization")).To(Equal("custom-auth"))
		Expect(req.Header.Get("X-Bk-Tenant-Id")).To(Equal("custom-tenant"))
	})

	It("lets custom User-Agent override the default value", func() {
		r := &api.Request{
			Method: "GET",
			URL:    "https://example.com/api",
			Headers: map[string]string{
				"User-Agent": "custom-client/9.9.9",
			},
		}
		req, err := r.Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(req.Header.Get("User-Agent")).To(Equal("custom-client/9.9.9"))
	})

	It("adds query params from JSON", func() {
		r := &api.Request{
			Method:     "GET",
			URL:        "https://example.com/api",
			ParamsJSON: `{"page":"1","size":"10"}`,
		}
		req, err := r.Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(req.URL.Query().Get("page")).To(Equal("1"))
		Expect(req.URL.Query().Get("size")).To(Equal("10"))
	})

	It("rejects URLs without an http or https scheme", func() {
		r := &api.Request{
			Method: "GET",
			URL:    "ftp://example.com/api",
		}

		_, err := r.Build()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("http or https"))
	})

	It("rejects URLs without a host", func() {
		r := &api.Request{
			Method: "GET",
			URL:    "https:///api",
		}

		_, err := r.Build()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("host"))
	})
})

var _ = Describe("ParseHeaderFlags", func() {
	It("parses Key:Value correctly", func() {
		result, err := api.ParseHeaderFlags([]string{"Content-Type:application/json", "Accept:text/html"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveKeyWithValue("Content-Type", "application/json"))
		Expect(result).To(HaveKeyWithValue("Accept", "text/html"))
	})

	It("handles spaces around key and value", func() {
		result, err := api.ParseHeaderFlags([]string{" X-Custom : my value "})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveKeyWithValue("X-Custom", "my value"))
	})

	It("rejects missing separators", func() {
		_, err := api.ParseHeaderFlags([]string{"missing-separator"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("expected key:value"))
	})

	It("rejects empty header names", func() {
		_, err := api.ParseHeaderFlags([]string{":value"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("header name cannot be empty"))
	})

	It("rejects invalid header names", func() {
		_, err := api.ParseHeaderFlags([]string{"Bad Header:value"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid header name"))
	})

	It("rejects invalid header values", func() {
		_, err := api.ParseHeaderFlags([]string{"X-Test:bad\nvalue"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid header value"))
	})
})

var _ = Describe("SubstitutePath", func() {
	It("substitutes a single string placeholder", func() {
		result, err := api.SubstitutePath("/api/v2/{biz_id}/sets/", `{"biz_id":"42"}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("/api/v2/42/sets/"))
	})

	It("substitutes an integer placeholder", func() {
		result, err := api.SubstitutePath("/api/v2/{biz_id}/", `{"biz_id":99}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("/api/v2/99/"))
	})

	It("preserves large integers without scientific notation", func() {
		result, err := api.SubstitutePath("/api/v2/{biz_id}/", `{"biz_id":1000000}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("/api/v2/1000000/"))
	})

	It("rejects object values for placeholders", func() {
		_, err := api.SubstitutePath("/api/{id}/", `{"id":{"nested":true}}`)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid --path value for \"id\""))
	})

	It("substitutes multiple placeholders", func() {
		result, err := api.SubstitutePath("/api/{a}/x/{b}/", `{"a":"hello","b":"world"}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("/api/hello/x/world/"))
	})

	It("escapes placeholder values as a single path segment", func() {
		result, err := api.SubstitutePath("/api/{id}/", `{"id":"a/b?c=d e%f"}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("/api/a%2Fb%3Fc=d%20e%25f/"))
	})

	It("rejects invalid gateway_name placeholder values", func() {
		_, err := api.SubstitutePath(
			"/api/v2/open/gateways/{gateway_name}/resources/",
			`{"gateway_name":"bk-iam/extra?x=1"}`,
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("gateway_name"))
	})

	It("returns path unchanged when pathJSON is empty and no placeholders", func() {
		result, err := api.SubstitutePath("/api/v2/things/", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("/api/v2/things/"))
	})

	It("errors on unresolved placeholder when pathJSON is empty", func() {
		_, err := api.SubstitutePath("/api/{id}/", "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unresolved placeholder"))
		Expect(err.Error()).To(ContainSubstring("id"))
	})

	It("errors on missing key for placeholder", func() {
		_, err := api.SubstitutePath("/api/{id}/", `{"name":"foo"}`)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unresolved placeholder"))
	})

	It("errors on extra keys not matching any placeholder", func() {
		_, err := api.SubstitutePath("/api/{id}/", `{"id":"1","extra":"2"}`)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("extra"))
		Expect(err.Error()).To(ContainSubstring("does not match any placeholder"))
	})

	It("errors on invalid JSON", func() {
		_, err := api.SubstitutePath("/api/{id}/", `{bad json}`)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid --path JSON"))
	})
})
