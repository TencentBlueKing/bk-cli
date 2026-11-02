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

package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GitHubSource", func() {
	BeforeEach(func() {
		originalClient := githubHTTPClient
		githubHTTPClient = &http.Client{}
		DeferCleanup(func() {
			githubHTTPClient = originalClient
		})
	})

	It("checks the latest release successfully", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.URL.Path).To(Equal("/repos/TencentBlueKing/bk-cli/releases/latest"))
			Expect(r.Header.Get("Accept")).To(Equal("application/vnd.github+json"))
			w.Header().Set("Content-Type", "application/json")
			assetName := expectedReleaseAssetName("v1.2.3", runtime.GOOS, runtime.GOARCH)
			_, _ = w.Write([]byte(`{
				"tag_name":"v1.2.3",
				"assets":[{"name":"` + assetName + `","browser_download_url":"https://example.com/` + assetName + `"}]
			}`))
		}))
		DeferCleanup(server.Close)

		githubHTTPClient = server.Client()

		source := &GitHubSource{Owner: "TencentBlueKing", Repo: "bk-cli", APIBase: server.URL}
		info, err := source.CheckLatest(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Version).To(Equal("1.2.3"))

		expected := "https://example.com/" + expectedReleaseAssetName("v1.2.3", runtime.GOOS, runtime.GOARCH)
		Expect(info.DownloadURL).To(Equal(expected))
	})

	It("matches GoReleaser asset names without a leading tag prefix", func() {
		assetName := expectedReleaseAssetName("v1.2.3", "linux", "amd64")
		Expect(assetName).To(Equal("bk-cli_1.2.3_linux_amd64.tar.gz"))

		assetName = expectedReleaseAssetName("1.2.3", "windows", "arm64")
		Expect(assetName).To(Equal("bk-cli_1.2.3_windows_arm64.zip"))
	})

	It("returns an error when the endpoint is not successful", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		DeferCleanup(server.Close)

		githubHTTPClient = server.Client()

		source := &GitHubSource{Owner: "TencentBlueKing", Repo: "bk-cli", APIBase: server.URL}
		_, err := source.CheckLatest(context.Background())
		Expect(err).To(MatchError(ContainSubstring("version check returned status 502")))
	})

	It("uses the default API endpoint and preserves transport errors", func() {
		transportErr := errors.New("transport unavailable")
		githubHTTPClient = &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				Expect(req.Method).To(Equal(http.MethodGet))
				Expect(req.URL.Scheme).To(Equal("https"))
				Expect(req.URL.Host).To(Equal("api.github.com"))
				Expect(req.URL.Path).To(Equal("/repos/TencentBlueKing/bk-cli/releases/latest"))
				return nil, transportErr
			}),
		}

		source := &GitHubSource{Owner: "TencentBlueKing", Repo: "bk-cli"}
		info, err := source.CheckLatest(context.Background())
		Expect(info).To(BeNil())
		Expect(err).To(MatchError(ContainSubstring("failed to check for updates")))
		Expect(errors.Is(err, transportErr)).To(BeTrue())
	})

	DescribeTable("rejects incomplete release metadata", func(payload, message string) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(payload))
		}))
		DeferCleanup(server.Close)
		githubHTTPClient = server.Client()

		source := &GitHubSource{Owner: "TencentBlueKing", Repo: "bk-cli", APIBase: server.URL}
		info, err := source.CheckLatest(context.Background())
		Expect(info).To(BeNil())
		Expect(err).To(MatchError(ContainSubstring(message)))
	},
		Entry("missing version tag", `{"assets":[]}`, "latest release has no tag name"),
		Entry("matching asset without a download URL",
			`{"tag_name":"v1.2.3","assets":[{"name":"`+
				expectedReleaseAssetName("v1.2.3", runtime.GOOS, runtime.GOARCH)+`"}]}`,
			"has no download URL"),
	)

	It("returns an error when the request URL is invalid", func() {
		source := &GitHubSource{Owner: "TencentBlueKing", Repo: "bk-cli", APIBase: "://bad-url"}
		_, err := source.CheckLatest(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create update request"))
	})

	It("returns an error when the payload is invalid JSON", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{bad`))
		}))
		DeferCleanup(server.Close)

		githubHTTPClient = server.Client()

		source := &GitHubSource{Owner: "TencentBlueKing", Repo: "bk-cli", APIBase: server.URL}
		_, err := source.CheckLatest(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to parse release info"))
	})

	It("returns an error when the platform asset is missing", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(
				[]byte(
					`{"tag_name":"v1.2.3","assets":[{"name":"bk-cli_v1.2.3_darwin_amd64.tar.gz","browser_download_url":"https://example.com/other"}]}`,
				),
			)
		}))
		DeferCleanup(server.Close)

		githubHTTPClient = server.Client()

		source := &GitHubSource{Owner: "TencentBlueKing", Repo: "bk-cli", APIBase: server.URL}
		_, err := source.CheckLatest(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("release asset"))
	})
})
