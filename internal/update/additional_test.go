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
	"net/http"
	"net/http/httptest"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GitHubSource", func() {
	BeforeEach(func() {
		githubHTTPClient = &http.Client{}
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
			_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","assets":[{"name":"bk-cli_v1.2.3_darwin_amd64.tar.gz","browser_download_url":"https://example.com/other"}]}`))
		}))
		DeferCleanup(server.Close)

		githubHTTPClient = server.Client()

		source := &GitHubSource{Owner: "TencentBlueKing", Repo: "bk-cli", APIBase: server.URL}
		_, err := source.CheckLatest(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("release asset"))
	})
})
