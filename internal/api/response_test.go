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
	"bytes"
	"net/http"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/api"
)

var _ = Describe("ExtractGatewayHeaders", func() {
	newResp := func(headers map[string]string) *http.Response {
		h := http.Header{}
		for k, v := range headers {
			h.Set(k, v)
		}
		return &http.Response{Header: h}
	}

	It("extracts X-Request-ID", func() {
		resp := newResp(map[string]string{"X-Request-Id": "req-123"})
		result := api.ExtractGatewayHeaders(resp)
		Expect(result).To(HaveKeyWithValue("X-Request-Id", "req-123"))
	})

	It("extracts X-Bkapi-Request-ID", func() {
		resp := newResp(map[string]string{"X-Bkapi-Request-Id": "bk-456"})
		result := api.ExtractGatewayHeaders(resp)
		Expect(result).To(HaveKeyWithValue("X-Bkapi-Request-Id", "bk-456"))
	})

	It("extracts all X-Bkapi-* headers", func() {
		resp := newResp(map[string]string{
			"X-Bkapi-Request-Id": "id1",
			"X-Bkapi-Something":  "val2",
		})
		result := api.ExtractGatewayHeaders(resp)
		Expect(result).To(HaveKeyWithValue("X-Bkapi-Request-Id", "id1"))
		Expect(result).To(HaveKeyWithValue("X-Bkapi-Something", "val2"))
	})

	It("includes key debug headers", func() {
		resp := newResp(map[string]string{
			"Content-Type":   "application/json",
			"Content-Length": "42",
			"ETag":           "abc",
			"X-Request-Id":   "req-789",
		})
		resp.StatusCode = http.StatusOK
		result := api.ExtractGatewayHeaders(resp)
		Expect(result).NotTo(HaveKey("Content-Type"))
		Expect(result).NotTo(HaveKey("Content-Length"))
		Expect(result).NotTo(HaveKey("ETag"))
		Expect(result).To(HaveKeyWithValue("X-Request-Id", "req-789"))
	})

	It("includes content-type and length for non-default success payloads", func() {
		resp := newResp(map[string]string{
			"Content-Type":   "text/plain; charset=utf-8",
			"Content-Length": "42",
		})
		resp.StatusCode = http.StatusOK
		result := api.ExtractGatewayHeaders(resp)
		Expect(result).To(HaveKeyWithValue("Content-Type", "text/plain; charset=utf-8"))
		Expect(result).To(HaveKeyWithValue("Content-Length", "42"))
	})

	It("includes zero content-length for successful empty responses", func() {
		resp := newResp(map[string]string{"Content-Length": "0"})
		resp.StatusCode = http.StatusNoContent
		result := api.ExtractGatewayHeaders(resp)
		Expect(result).To(HaveKeyWithValue("Content-Length", "0"))
	})

	It("includes debug headers on non-2xx responses", func() {
		resp := newResp(map[string]string{
			"Content-Type":   "application/json",
			"Content-Length": "0",
			"Traceparent":    "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00",
		})
		resp.StatusCode = http.StatusBadGateway
		result := api.ExtractGatewayHeaders(resp)
		Expect(result).To(HaveKeyWithValue("Content-Type", "application/json"))
		Expect(result).To(HaveKeyWithValue("Content-Length", "0"))
		Expect(
			result,
		).To(
			HaveKeyWithValue("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"),
		)
	})

	It("omits traceparent on normal successful responses", func() {
		resp := newResp(
			map[string]string{"Traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"},
		)
		resp.StatusCode = http.StatusOK
		result := api.ExtractGatewayHeaders(resp)
		Expect(result).NotTo(HaveKey("Traceparent"))
	})

	It("preserves canonical HTTP header keys", func() {
		resp := newResp(map[string]string{"X-Bkapi-My-Header": "test"})
		result := api.ExtractGatewayHeaders(resp)
		Expect(result).To(HaveKey("X-Bkapi-My-Header"))
	})
})

var _ = Describe("BuildDryRunEnvelope", func() {
	It("creates dry-run envelope with request details", func() {
		req := &api.Request{
			Method:     "POST",
			URL:        "https://example.com/api/v1/test",
			AuthHeader: `{"bk_app_code":"secret"}`,
			TenantID:   "t1",
			Headers:    map[string]string{"X-Custom": "val"},
			ParamsJSON: `{"a":"1"}`,
			BodyJSON:   `{"key":"value"}`,
		}
		env := api.BuildDryRunEnvelope(req)
		Expect(env.OK).To(BeTrue())
		Expect(env.DryRun).To(BeTrue())
		Expect(env.Request).NotTo(BeNil())
		Expect(env.Request.Method).To(Equal("POST"))
		Expect(env.Request.URL).To(Equal("https://example.com/api/v1/test"))
		Expect(env.Request.Headers).To(HaveKeyWithValue("User-Agent", "bk-cli/dev"))
		Expect(env.Request.Headers).To(HaveKeyWithValue("Content-Type", "application/json"))
		Expect(env.Request.Headers).To(HaveKeyWithValue("X-Bk-Tenant-Id", "t1"))

		params, ok := env.Request.Params.(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(params).To(HaveKeyWithValue("a", "1"))

		body, ok := env.Request.Body.(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(body).To(HaveKeyWithValue("key", "value"))
	})

	It("redacts auth header", func() {
		req := &api.Request{
			Method:     "GET",
			URL:        "https://example.com/api",
			AuthHeader: `{"bk_app_code":"secret","bk_app_secret":"topsecret"}`,
		}
		env := api.BuildDryRunEnvelope(req)
		Expect(env.Request.Headers).To(HaveKeyWithValue("X-Bkapi-Authorization", "{...redacted...}"))
	})

	It("preserves large integers in dry-run params and body", func() {
		req := &api.Request{
			Method:     "POST",
			URL:        "https://example.com/api",
			ParamsJSON: `{"job_instance_id":20004841045,"ids":[1,20004841045]}`,
			BodyJSON:   `{"id":20004841045,"nested":{"values":[20004841045]}}`,
		}

		env := api.BuildDryRunEnvelope(req)

		params, ok := env.Request.Params.(map[string]any)
		Expect(ok).To(BeTrue())
		jobInstanceID, ok := params["job_instance_id"].(json.Number)
		Expect(ok).To(BeTrue())
		Expect(jobInstanceID.String()).To(Equal("20004841045"))
		ids, ok := params["ids"].([]any)
		Expect(ok).To(BeTrue())
		idValue, ok := ids[1].(json.Number)
		Expect(ok).To(BeTrue())
		Expect(idValue.String()).To(Equal("20004841045"))

		body, ok := env.Request.Body.(map[string]any)
		Expect(ok).To(BeTrue())
		bodyID, ok := body["id"].(json.Number)
		Expect(ok).To(BeTrue())
		Expect(bodyID.String()).To(Equal("20004841045"))
		nested, ok := body["nested"].(map[string]any)
		Expect(ok).To(BeTrue())
		values, ok := nested["values"].([]any)
		Expect(ok).To(BeTrue())
		nestedValue, ok := values[0].(json.Number)
		Expect(ok).To(BeTrue())
		Expect(nestedValue.String()).To(Equal("20004841045"))

		var out bytes.Buffer
		Expect(env.WriteJSON(&out)).To(Succeed())
		Expect(out.String()).To(ContainSubstring(`"job_instance_id": 20004841045`))
		Expect(out.String()).To(ContainSubstring(`"id": 20004841045`))
	})
})
