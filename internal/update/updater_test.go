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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// makeTarGz creates a tar.gz archive containing a single file named bk-cli with the given content.
func makeTarGz(content []byte) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	_ = tw.WriteHeader(&tar.Header{
		Name:     "bk-cli",
		Size:     int64(len(content)),
		Mode:     0o755,
		Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write(content)
	_ = tw.Close()
	_ = gw.Close()

	return buf.Bytes()
}

var _ = Describe("updater", func() {
	BeforeEach(func() {
		downloadHTTPClient = &http.Client{}
		executablePath = os.Executable
		execCommand = exec.CommandContext
	})

	It("compares semantic versions correctly", func() {
		Expect(IsNewer("1.2.3", "1.2.4")).To(BeTrue())
		Expect(IsNewer("v1.2.3", "1.2.3")).To(BeFalse())
		Expect(IsNewer("1.2.3", "not-semver")).To(BeTrue())
		Expect(IsNewer("not-semver", "not-semver")).To(BeFalse())
	})

	It("downloads tar.gz and replaces the executable while preserving permissions", func() {
		tarGzData := makeTarGz([]byte("new-binary"))

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(tarGzData)
		}))
		DeferCleanup(server.Close)

		dir := GinkgoT().TempDir()
		target := filepath.Join(dir, "bk-cli")
		Expect(os.WriteFile(target, []byte("old-binary"), 0o755)).To(Succeed())

		downloadHTTPClient = server.Client()
		executablePath = func() (string, error) {
			return target, nil
		}

		Expect(DownloadAndReplace(server.URL)).To(Succeed())

		data, err := os.ReadFile(target)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("new-binary"))

		info, err := os.Stat(target)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o755)))
	})

	It("detects npm-installed binaries", func() {
		executablePath = func() (string, error) {
			return "/usr/local/lib/node_modules/@blueking/bk-cli/bin/bk-cli", nil
		}
		Expect(IsNPMInstall()).To(BeTrue())
	})

	It("detects non-npm-installed binaries", func() {
		executablePath = func() (string, error) {
			return "/usr/local/bin/bk-cli", nil
		}
		Expect(IsNPMInstall()).To(BeFalse())
	})

	It("returns false for npm detection when executable path fails", func() {
		executablePath = func() (string, error) {
			return "", os.ErrNotExist
		}
		Expect(IsNPMInstall()).To(BeFalse())
	})

	It("runs npm update successfully", func() {
		execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "echo", "updated @blueking/bk-cli")
		}
		out, err := RunNPMUpdate(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(string(out)).To(ContainSubstring("updated @blueking/bk-cli"))
	})

	It("returns an error when npm update fails", func() {
		execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "false")
		}
		_, err := RunNPMUpdate(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("npm update failed"))
	})

	It("returns a download error when the server responds with a non-200 status", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		DeferCleanup(server.Close)

		downloadHTTPClient = server.Client()

		err := DownloadAndReplace(server.URL)
		Expect(err).To(MatchError(ContainSubstring("download returned status 500")))
	})

	It("returns an error when the download URL is invalid", func() {
		err := DownloadAndReplace("://bad-url")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create download request"))
	})

	It("returns an error when the executable cannot be statted", func() {
		tarGzData := makeTarGz([]byte("new-binary"))

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(tarGzData)
		}))
		DeferCleanup(server.Close)

		downloadHTTPClient = server.Client()
		executablePath = func() (string, error) {
			return filepath.Join(GinkgoT().TempDir(), "missing-bk-cli"), nil
		}

		err := DownloadAndReplace(server.URL)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cannot stat executable"))
	})

	It("returns a wrapped download error when the HTTP client fails", func() {
		downloadHTTPClient = &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("dial failed")
			}),
		}

		err := DownloadAndReplace("https://example.com/bk-cli.tar.gz")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("download failed"))
	})

	It("returns an error when the download body is not valid gzip", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-gzip-data"))
		}))
		DeferCleanup(server.Close)

		downloadHTTPClient = server.Client()
		executablePath = func() (string, error) {
			target := filepath.Join(GinkgoT().TempDir(), "bk-cli")
			Expect(os.WriteFile(target, []byte("old-binary"), 0o755)).To(Succeed())
			return target, nil
		}

		err := DownloadAndReplace(server.URL)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to open gzip stream"))
	})

	It("returns an error when bk-cli binary is not found in archive", func() {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)
		content := []byte("other-binary")
		_ = tw.WriteHeader(&tar.Header{
			Name:     "other-file",
			Size:     int64(len(content)),
			Mode:     0o755,
			Typeflag: tar.TypeReg,
		})
		_, _ = tw.Write(content)
		_ = tw.Close()
		_ = gw.Close()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(buf.Bytes())
		}))
		DeferCleanup(server.Close)

		downloadHTTPClient = server.Client()
		executablePath = func() (string, error) {
			target := filepath.Join(GinkgoT().TempDir(), "bk-cli")
			Expect(os.WriteFile(target, []byte("old-binary"), 0o755)).To(Succeed())
			return target, nil
		}

		err := DownloadAndReplace(server.URL)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("bk-cli binary not found in archive"))
	})

	It("returns an error when the executable path cannot be determined", func() {
		tarGzData := makeTarGz([]byte("new-binary"))

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(tarGzData)
		}))
		DeferCleanup(server.Close)

		downloadHTTPClient = server.Client()
		executablePath = func() (string, error) {
			return "", errors.New("no executable")
		}

		err := DownloadAndReplace(server.URL)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cannot determine executable path"))
	})

	It("returns an error when replacing the executable path fails", func() {
		tarGzData := makeTarGz([]byte("new-binary"))

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(tarGzData)
		}))
		DeferCleanup(server.Close)

		targetDir := filepath.Join(GinkgoT().TempDir(), "bk-cli-dir")
		Expect(os.MkdirAll(targetDir, 0o755)).To(Succeed())

		downloadHTTPClient = server.Client()
		executablePath = func() (string, error) {
			return targetDir, nil
		}

		err := DownloadAndReplace(server.URL)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("replace failed"))
	})
})

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
