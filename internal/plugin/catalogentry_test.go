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

package plugin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BuildReleaseEntry", func() {
	serve := func(files map[string][]byte) *httptest.Server {
		s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, ok := files[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(body)
		}))
		DeferCleanup(s.Close)
		return s
	}

	testClient := func(server *httptest.Server) *http.Client {
		client := server.Client()
		client.CheckRedirect = httpsOnlyRedirectPolicy
		return client
	}

	It("verifies archives against checksums.txt and hashes the extracted executable", func() {
		linux := archiveFixture("tar.gz", []archiveEntry{{Name: "bkms-cli", Body: "linux-bin", Mode: 0o755}})
		win := archiveFixture("zip", []archiveEntry{{Name: "bkms-cli.exe", Body: "win-bin", Mode: 0o755}})
		checksums := fmt.Sprintf(
			"%s  bkms-cli_1.0.4_linux_amd64.tar.gz\n%s  bkms-cli_1.0.4_windows_amd64.zip\n",
			digest(linux), digest(win),
		)
		s := serve(map[string][]byte{
			"/rel/checksums.txt":                     []byte(checksums),
			"/rel/bkms-cli_1.0.4_linux_amd64.tar.gz": linux,
			"/rel/bkms-cli_1.0.4_windows_amd64.zip":  win,
		})
		rel, err := BuildReleaseEntry(context.Background(), testClient(s), EntryOptions{
			ReleaseBaseURL: s.URL + "/rel", AssetTemplate: "bkms-cli_{version}_{os}_{arch}",
			Executable: "bkms-cli", Version: "v1.0.4", Auth: "none",
			Platforms: []string{"linux-amd64", "windows-amd64"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(rel.Status).To(Equal("allowed"))
		Expect(rel.Platforms["linux-amd64"]).To(Equal(Asset{
			URL: s.URL + "/rel/bkms-cli_1.0.4_linux_amd64.tar.gz", ArchiveSHA256: digest(linux),
			Executable: "bkms-cli", ExecutableSHA256: digest([]byte("linux-bin")),
		}))
		Expect(rel.Platforms["windows-amd64"].Executable).To(Equal("bkms-cli.exe"))
	})

	It("fails when an archive does not match checksums.txt", func() {
		linux := archiveFixture("tar.gz", []archiveEntry{{Name: "bkms-cli", Body: "linux-bin", Mode: 0o755}})
		s := serve(map[string][]byte{
			"/rel/checksums.txt": []byte(
				digest([]byte("other")) + "  bkms-cli_1.0.4_linux_amd64.tar.gz\n",
			),
			"/rel/bkms-cli_1.0.4_linux_amd64.tar.gz": linux,
		})
		_, err := BuildReleaseEntry(context.Background(), testClient(s), EntryOptions{
			ReleaseBaseURL: s.URL + "/rel", AssetTemplate: "bkms-cli_{version}_{os}_{arch}",
			Executable: "bkms-cli", Version: "v1.0.4", Auth: "none", Platforms: []string{"linux-amd64"},
		})
		expectPluginError(err, CodeDigestMismatch)
	})

	It("fails when checksums.txt lacks an archive", func() {
		s := serve(map[string][]byte{"/rel/checksums.txt": []byte("")})
		_, err := BuildReleaseEntry(context.Background(), testClient(s), EntryOptions{
			ReleaseBaseURL: s.URL + "/rel", AssetTemplate: "bkms-cli_{version}_{os}_{arch}",
			Executable: "bkms-cli", Version: "v1.0.4", Auth: "none", Platforms: []string{"linux-amd64"},
		})
		expectPluginError(err, CodeDownloadFailed)
	})

	It("rejects redirects to non-https URLs", func() {
		s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://evil.example/checksums.txt", http.StatusFound)
		}))
		DeferCleanup(s.Close)
		_, err := BuildReleaseEntry(context.Background(), testClient(s), EntryOptions{
			ReleaseBaseURL: s.URL + "/rel", AssetTemplate: "bkms-cli_{version}_{os}_{arch}",
			Executable: "bkms-cli", Version: "v1.0.4", Auth: "none", Platforms: []string{"linux-amd64"},
		})
		expectPluginError(err, CodeDownloadFailed)
	})

	DescribeTable(
		"rejects unsafe executable names",
		func(executable string) {
			_, err := BuildReleaseEntry(context.Background(), nil, EntryOptions{
				ReleaseBaseURL: "https://example.com/rel",
				AssetTemplate:  "bkms-cli_{version}_{os}_{arch}",
				Executable:     executable,
				Version:        "v1.0.4",
				Auth:           "none",
				Platforms:      []string{"linux-amd64"},
			})
			expectPluginError(err, CodeCatalogInvalid)
		},
		Entry("empty", ""),
		Entry("dot", "."),
		Entry("dotdot", ".."),
		Entry("path separator", "bin/bkms-cli"),
		Entry("backslash", `bin\bkms-cli`),
		Entry("parent traversal", "../bkms-cli"),
	)

	DescribeTable(
		"rejects unsafe asset templates",
		func(template string) {
			_, err := BuildReleaseEntry(context.Background(), nil, EntryOptions{
				ReleaseBaseURL: "https://example.com/rel",
				AssetTemplate:  template,
				Executable:     "bkms-cli",
				Version:        "v1.0.4",
				Auth:           "none",
				Platforms:      []string{"linux-amd64"},
			})
			expectPluginError(err, CodeCatalogInvalid)
		},
		Entry("parent traversal", "../outside_{os}_{arch}"),
		Entry("path separator", "a/b_{os}_{arch}"),
	)

	DescribeTable(
		"rejects unsupported platforms",
		func(platform string) {
			_, err := BuildReleaseEntry(context.Background(), nil, EntryOptions{
				ReleaseBaseURL: "https://example.com/rel",
				AssetTemplate:  "bkms-cli_{version}_{os}_{arch}",
				Executable:     "bkms-cli",
				Version:        "v1.0.4",
				Auth:           "none",
				Platforms:      []string{platform},
			})
			expectPluginError(err, CodeCatalogInvalid)
		},
		Entry("unknown os", "freebsd-amd64"),
		Entry("missing arch", "linux"),
		Entry("extra segment", "linux-amd64-extra"),
		Entry("path separator", "linux/amd64"),
	)
})
