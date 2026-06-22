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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/api"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
)

type errReadCloser struct{}

func (e errReadCloser) Read(_ []byte) (int, error) {
	return 0, errors.New("read failed")
}

func (e errReadCloser) Close() error {
	return nil
}

var _ = Describe("additional api coverage", func() {
	It("uses AuthRequirements defaults when nil", func() {
		var authConfig *api.AuthRequirements
		Expect(authConfig.RequiresAuth()).To(BeTrue())
		Expect(authConfig.RequiresAppVerification()).To(BeTrue())
		Expect(authConfig.RequiresUserVerification()).To(BeTrue())
	})

	It("allows app-only auth requirements", func() {
		authConfig := &api.AuthRequirements{AppVerifiedRequired: true}
		Expect(authConfig.RequiresAuth()).To(BeTrue())
		Expect(authConfig.RequiresAppVerification()).To(BeTrue())
		Expect(authConfig.RequiresUserVerification()).To(BeFalse())
	})

	It("executes validated HTTP requests", func() {
		api.SetUserAgentVersion("v2.3.4")
		DeferCleanup(api.SetUserAgentVersion, "dev")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Header.Get("User-Agent")).To(Equal("bk-cli/v2.3.4"))
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		DeferCleanup(server.Close)

		req, err := (&api.Request{
			Method: http.MethodGet,
			URL:    server.URL,
		}).Build()
		Expect(err).NotTo(HaveOccurred())

		resp, err := api.NewClient(0).Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Body.Close()).To(Succeed())
	})

	It("rejects invalid request URLs before dispatch", func() {
		req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
		Expect(err).NotTo(HaveOccurred())
		req.URL.Scheme = "ftp"

		_, err = api.NewClient(0).Do(req)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("http or https"))
	})

	It("parses JSON, plain text, and empty responses", func() {
		jsonResp := &http.Response{Body: io.NopCloser(strings.NewReader(`{"value":1}`))}
		parsed, err := api.ParseResponse(jsonResp)
		Expect(err).NotTo(HaveOccurred())
		Expect(parsed).To(Equal(map[string]any{"value": float64(1)}))

		textResp := &http.Response{Body: io.NopCloser(strings.NewReader("plain-text"))}
		parsed, err = api.ParseResponse(textResp)
		Expect(err).NotTo(HaveOccurred())
		Expect(parsed).To(Equal("plain-text"))

		emptyResp := &http.Response{Body: io.NopCloser(strings.NewReader(""))}
		parsed, err = api.ParseResponse(emptyResp)
		Expect(err).NotTo(HaveOccurred())
		Expect(parsed).To(BeNil())
	})

	It("returns a response read error", func() {
		_, err := api.ParseResponse(&http.Response{Body: errReadCloser{}})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to read response body"))
	})

	It("builds response envelopes from HTTP responses", func() {
		resp := &http.Response{
			StatusCode: http.StatusAccepted,
			Header:     http.Header{"X-Request-Id": []string{"req-123"}},
			Body:       io.NopCloser(strings.NewReader(`{"queued":true}`)),
		}

		env, err := api.BuildResponseEnvelope(resp)
		Expect(err).NotTo(HaveOccurred())
		Expect(env.Status).To(Equal(http.StatusAccepted))
		Expect(env.Headers).To(HaveKeyWithValue("X-Request-Id", "req-123"))
		Expect(env.Data).To(Equal(map[string]any{"queued": true}))
	})

	It("returns response envelope errors when the body cannot be read", func() {
		_, err := api.BuildResponseEnvelope(&http.Response{Body: errReadCloser{}})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to read response body"))
	})

	It("rejects invalid query JSON before adding query params", func() {
		req, err := http.NewRequest(http.MethodGet, "https://example.com/api", nil)
		Expect(err).NotTo(HaveOccurred())

		err = api.AddQueryParams(req, `{bad`)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid --query JSON"))
	})

	It("preserves large integer query params without scientific notation", func() {
		req, err := http.NewRequest(http.MethodGet, "https://example.com/api", nil)
		Expect(err).NotTo(HaveOccurred())

		err = api.AddQueryParams(req, `{"bk_biz_id":2,"job_instance_id":20004841045,"return_ip_result":true}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(req.URL.Query().Get("bk_biz_id")).To(Equal("2"))
		Expect(req.URL.Query().Get("job_instance_id")).To(Equal("20004841045"))
		Expect(req.URL.Query().Get("return_ip_result")).To(Equal("true"))
	})

	It("keeps raw strings in dry-run payloads when JSON decoding fails", func() {
		env := api.BuildDryRunEnvelope(&api.Request{
			Method:     "POST",
			URL:        "https://example.com/api",
			ParamsJSON: `{bad`,
			BodyJSON:   `{bad`,
		})

		Expect(env.Request.Params).To(Equal(`{bad`))
		Expect(env.Request.Body).To(Equal(`{bad`))
	})

	It("rejects invalid JSON bodies while building requests", func() {
		_, err := (&api.Request{
			Method:   http.MethodPost,
			URL:      "https://example.com/api",
			BodyJSON: `{bad`,
		}).Build()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid --body JSON"))
	})

	It("returns auth header errors for incomplete credentials", func() {
		_, err := api.BuildAuthHeader(&credential.Credential{
			Type:        credential.TypeAppUser,
			BkAppSecret: "secret",
			BkToken:     "token",
		}, &api.AuthRequirements{AppVerifiedRequired: true, UserVerifiedRequired: true})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("bk_app_code is required"))

		_, err = api.BuildAuthHeader(&credential.Credential{
			Type: credential.TypeAccessToken,
		}, &api.AuthRequirements{AppVerifiedRequired: true})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("access_token is required"))
	})
})
