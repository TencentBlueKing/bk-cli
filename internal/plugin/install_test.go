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
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type archiveEntry struct {
	Name     string
	Body     string
	Mode     fs.FileMode
	Symlink  string
	Hardlink string
}

func archiveFixture(format string, entries []archiveEntry) []byte {
	var out bytes.Buffer
	if format == "zip" {
		zw := zip.NewWriter(&out)
		for _, e := range entries {
			h := &zip.FileHeader{Name: e.Name, Method: zip.Deflate}
			h.SetMode(e.Mode)
			body := e.Body
			if e.Symlink != "" {
				h.SetMode(fs.ModeSymlink | 0o777)
				body = e.Symlink
			}
			w, err := zw.CreateHeader(h)
			Expect(err).NotTo(HaveOccurred())
			_, err = w.Write([]byte(body))
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(zw.Close()).To(Succeed())
		return out.Bytes()
	}
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{
			Name: e.Name, Mode: int64(e.Mode.Perm()), Size: int64(len(e.Body)), Typeflag: tar.TypeReg,
		}
		switch {
		case e.Symlink != "":
			h.Typeflag, h.Linkname, h.Size = tar.TypeSymlink, e.Symlink, 0
		case e.Hardlink != "":
			h.Typeflag, h.Linkname, h.Size = tar.TypeLink, e.Hardlink, 0
		}
		Expect(tw.WriteHeader(h)).To(Succeed())
		if h.Typeflag == tar.TypeReg {
			_, err := tw.Write([]byte(e.Body))
			Expect(err).NotTo(HaveOccurred())
		}
	}
	Expect(tw.Close()).To(Succeed())
	Expect(gz.Close()).To(Succeed())
	return out.Bytes()
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type releaseFixture struct {
	Version string
	Archive []byte
	Binary  []byte
}

// testManager builds an in-memory catalog; it never touches the embedded catalog.
func testManager(base, platform, serverURL string, releases ...releaseFixture) *Manager {
	exe := "bkms-cli"
	if ArchiveFormat(platform) == "zip" {
		exe += ".exe"
	}
	versions := map[string]Release{}
	for _, r := range releases {
		versions[r.Version] = Release{
			Status: "allowed",
			Auth:   "none",
			Platforms: map[string]Asset{
				platform: {
					URL:              serverURL + "/" + r.Version + "." + ArchiveFormat(platform),
					ArchiveSHA256:    digest(r.Archive),
					Executable:       exe,
					ExecutableSHA256: digest(r.Binary),
				},
			},
		}
	}
	c := &Catalog{
		SchemaVersion: 1,
		Plugins: map[string]Definition{
			"bkms": {
				Binary:             "bkms-cli",
				RecommendedVersion: releases[0].Version,
				Versions:           versions,
			},
		},
	}
	goos, goarch, _ := strings.Cut(platform, "-")
	return NewManager(c, base, goos, goarch)
}

func serveReleases(releases ...releaseFixture) (*httptest.Server, *atomic.Int32) {
	var calls atomic.Int32
	byPath := map[string][]byte{}
	for _, r := range releases {
		byPath["/"+r.Version+".tar.gz"] = r.Archive
		byPath["/"+r.Version+".zip"] = r.Archive
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, ok := byPath[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	DeferCleanup(server.Close)
	return server, &calls
}

func tarRelease(version, body string) releaseFixture {
	bin := []byte(body)
	return releaseFixture{
		Version: version,
		Binary:  bin,
		Archive: archiveFixture("tar.gz", []archiveEntry{
			{Name: "bkms-cli", Body: body, Mode: 0o755},
			{Name: "README.md", Body: "doc", Mode: 0o644},
		}),
	}
}

var _ = Describe("plugin installation", func() {
	var base string
	BeforeEach(func() { base = GinkgoT().TempDir() })

	It("installs the recommended version, then is idempotent without downloading", func() {
		rel := tarRelease("v1.0.4", "inert fixture v1.0.4")
		server, calls := serveReleases(rel)
		m := testManager(base, "linux-amd64", server.URL, rel)
		m.Client = server.Client()

		first, err := m.Install(context.Background(), "bkms", InstallOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Changed).To(BeTrue())
		Expect(first.Path).To(Equal(
			filepath.Join(base, "plugins", "bkms", "v1.0.4", "linux-amd64", "bkms-cli"),
		))
		Expect(os.ReadFile(first.Path)).To(Equal(rel.Binary))
		Expect(m.InstalledVersion("bkms")).To(Equal("v1.0.4"))

		second, err := m.Install(context.Background(), "bkms", InstallOptions{Version: "v1.0.4"})
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Changed).To(BeFalse())
		Expect(calls.Load()).To(Equal(int32(1)))
		_, err = os.Stat(filepath.Join(base, "plugins", "README.md"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	It("switches versions and keeps exactly one version on disk", func() {
		older, newer := tarRelease("v1.0.3", "old"), tarRelease("v1.0.4", "new")
		server, _ := serveReleases(older, newer)
		m := testManager(base, "linux-amd64", server.URL, newer, older)
		m.Client = server.Client()

		_, err := m.Install(context.Background(), "bkms", InstallOptions{Version: "v1.0.3"})
		Expect(err).NotTo(HaveOccurred())
		res, err := m.Update(context.Background(), "bkms")
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Version).To(Equal("v1.0.4"))
		entries, err := os.ReadDir(filepath.Join(base, "plugins", "bkms"))
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].Name()).To(Equal("v1.0.4"))
	})

	It("installs from a local archive with the same verification and no HTTP", func() {
		rel := tarRelease("v1.0.4", "offline fixture")
		server, calls := serveReleases(rel)
		m := testManager(base, "linux-amd64", server.URL, rel)
		file := filepath.Join(GinkgoT().TempDir(), "bkms.tar.gz")
		Expect(os.WriteFile(file, rel.Archive, 0o600)).To(Succeed())

		_, err := m.Install(context.Background(), "bkms", InstallOptions{FromFile: file})
		Expect(err).NotTo(HaveOccurred())
		Expect(calls.Load()).To(BeZero())

		Expect(os.WriteFile(file, []byte("tampered"), 0o600)).To(Succeed())
		Expect(m.Remove("bkms")).To(BeTrue())
		_, err = m.Install(context.Background(), "bkms", InstallOptions{FromFile: file})
		expectPluginError(err, CodeDigestMismatch)
	})

	It("installs windows zip archives", func() {
		bin := []byte("zip fixture")
		rel := releaseFixture{
			Version: "v1.0.4",
			Binary:  bin,
			Archive: archiveFixture(
				"zip",
				[]archiveEntry{{Name: "bkms-cli.exe", Body: string(bin), Mode: 0o755}},
			),
		}
		server, _ := serveReleases(rel)
		m := testManager(base, "windows-amd64", server.URL, rel)
		m.Client = server.Client()
		res, err := m.Install(context.Background(), "bkms", InstallOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(filepath.Base(res.Path)).To(Equal("bkms-cli.exe"))
	})

	It("detects a replaced executable and reinstalls it on the next install", func() {
		rel := tarRelease("v1.0.4", "original")
		server, calls := serveReleases(rel)
		m := testManager(base, "linux-amd64", server.URL, rel)
		m.Client = server.Client()
		res, err := m.Install(context.Background(), "bkms", InstallOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(os.WriteFile(res.Path, []byte("#!/bin/sh\necho stolen\n"), 0o700)).To(Succeed())
		_, _, err = m.VerifiedExecutable("bkms")
		expectPluginError(err, CodeDigestMismatch)

		again, err := m.Install(context.Background(), "bkms", InstallOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(again.Changed).To(BeTrue())
		Expect(calls.Load()).To(Equal(int32(2)))
		_, _, err = m.VerifiedExecutable("bkms")
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects a symlinked executable even if the target matches", func() {
		rel := tarRelease("v1.0.4", "original")
		server, _ := serveReleases(rel)
		m := testManager(base, "linux-amd64", server.URL, rel)
		m.Client = server.Client()
		res, err := m.Install(context.Background(), "bkms", InstallOptions{})
		Expect(err).NotTo(HaveOccurred())
		copyPath := filepath.Join(GinkgoT().TempDir(), "copy")
		Expect(os.WriteFile(copyPath, rel.Binary, 0o700)).To(Succeed())
		Expect(os.Remove(res.Path)).To(Succeed())
		if err := os.Symlink(copyPath, res.Path); err != nil {
			Skip("symlinks unavailable: " + err.Error())
		}
		_, _, err = m.VerifiedExecutable("bkms")
		expectPluginError(err, CodeDigestMismatch)
	})

	It("reports not installed and does not look at PATH", func() {
		m := testManager(base, "linux-amd64", "https://unused.invalid", tarRelease("v1.0.4", "x"))
		_, _, err := m.VerifiedExecutable("bkms")
		expectPluginError(err, CodeNotInstalled)
	})

	It("rejects an installed version that the catalog no longer allows", func() {
		rel := tarRelease("v1.0.4", "x")
		server, _ := serveReleases(rel)
		m := testManager(base, "linux-amd64", server.URL, rel)
		m.Client = server.Client()
		_, err := m.Install(context.Background(), "bkms", InstallOptions{})
		Expect(err).NotTo(HaveOccurred())
		m.Catalog.Plugins["bkms"].Versions["v1.0.4"] = Release{Status: "revoked"}
		_, _, err = m.VerifiedExecutable("bkms")
		expectPluginError(err, CodeVersionNotAllowed)
	})

	DescribeTable(
		"keeps the previous install when a new install fails",
		func(mutate func(*Manager, *releaseFixture), code string) {
			good := tarRelease("v1.0.3", "good")
			bad := tarRelease("v1.0.4", "bad")
			server, _ := serveReleases(good, bad)
			m := testManager(base, "linux-amd64", server.URL, good, bad)
			m.Client = server.Client()
			_, err := m.Install(context.Background(), "bkms", InstallOptions{Version: "v1.0.3"})
			Expect(err).NotTo(HaveOccurred())
			mutate(m, &bad)

			_, err = m.Install(context.Background(), "bkms", InstallOptions{Version: "v1.0.4"})
			expectPluginError(err, code)
			Expect(m.InstalledVersion("bkms")).To(Equal("v1.0.3"))
			_, _, err = m.VerifiedExecutable("bkms")
			Expect(err).NotTo(HaveOccurred())
			leftovers, _ := filepath.Glob(filepath.Join(base, "plugins", ".staging-*"))
			Expect(leftovers).To(BeEmpty())
		},
		Entry("404", func(m *Manager, _ *releaseFixture) {
			a := m.Catalog.Plugins["bkms"].Versions["v1.0.4"].Platforms["linux-amd64"]
			a.URL += ".missing"
			m.Catalog.Plugins["bkms"].Versions["v1.0.4"].Platforms["linux-amd64"] = a
		}, CodeDownloadFailed),
		Entry("archive digest", func(m *Manager, _ *releaseFixture) {
			a := m.Catalog.Plugins["bkms"].Versions["v1.0.4"].Platforms["linux-amd64"]
			a.ArchiveSHA256 = digest([]byte("other"))
			m.Catalog.Plugins["bkms"].Versions["v1.0.4"].Platforms["linux-amd64"] = a
		}, CodeDigestMismatch),
		Entry("executable digest", func(m *Manager, _ *releaseFixture) {
			a := m.Catalog.Plugins["bkms"].Versions["v1.0.4"].Platforms["linux-amd64"]
			a.ExecutableSHA256 = digest([]byte("other"))
			m.Catalog.Plugins["bkms"].Versions["v1.0.4"].Platforms["linux-amd64"] = a
		}, CodeDigestMismatch),
	)

	It("returns busy while another operation holds the lock", func() {
		rel := tarRelease("v1.0.4", "x")
		m := testManager(base, "linux-amd64", "https://unused.invalid", rel)
		pluginsDir := filepath.Join(base, "plugins")
		lockPath := filepath.Join(pluginsDir, ".lock")
		Expect(os.MkdirAll(pluginsDir, 0o700)).To(Succeed())
		Expect(os.WriteFile(lockPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600)).To(Succeed())
		_, err := m.Install(context.Background(), "bkms", InstallOptions{})
		pErr := expectPluginError(err, CodeBusy)
		Expect(pErr.Hint).To(ContainSubstring(lockPath))
	})

	It("takes over a lock held by a dead process", func() {
		rel := tarRelease("v1.0.4", "x")
		m := testManager(base, "linux-amd64", "https://unused.invalid", rel)
		pluginsDir := filepath.Join(base, "plugins")
		lockPath := filepath.Join(pluginsDir, ".lock")
		const deadPID = 2147483647
		Expect(processAlive(deadPID)).To(BeFalse())
		Expect(os.MkdirAll(pluginsDir, 0o700)).To(Succeed())
		Expect(os.WriteFile(lockPath, []byte(fmt.Sprintf("%d\n", deadPID)), 0o600)).To(Succeed())

		unlock, err := m.lock()
		Expect(err).NotTo(HaveOccurred())
		defer unlock()
		Expect(os.ReadFile(lockPath)).To(Equal([]byte(fmt.Sprintf("%d\n", os.Getpid()))))
	})

	It("cleans leftover staging directories after acquiring the lock", func() {
		rel := tarRelease("v1.0.4", "x")
		m := testManager(base, "linux-amd64", "https://unused.invalid", rel)
		staging := filepath.Join(base, "plugins", ".staging-leftover")
		Expect(os.MkdirAll(staging, 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(staging, "archive"), []byte("partial"), 0o600)).To(Succeed())

		unlock, err := m.lock()
		Expect(err).NotTo(HaveOccurred())
		defer unlock()
		_, err = os.Stat(staging)
		Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
	})

	It("checks the operation lock before reading state during update", func() {
		rel := tarRelease("v1.0.4", "x")
		m := testManager(base, "linux-amd64", "https://unused.invalid", rel)
		pluginsDir := filepath.Join(base, "plugins")
		Expect(os.MkdirAll(pluginsDir, 0o700)).To(Succeed())
		state := []byte("plugins: [corrupted")
		Expect(os.WriteFile(filepath.Join(pluginsDir, installedFileName), state, 0o600)).To(Succeed())
		Expect(os.WriteFile(
			filepath.Join(pluginsDir, ".lock"),
			[]byte(fmt.Sprintf("%d\n", os.Getpid())),
			0o600,
		)).To(Succeed())

		_, err := m.Update(context.Background(), "bkms")
		expectPluginError(err, CodeBusy)
		Expect(os.ReadFile(filepath.Join(pluginsDir, installedFileName))).To(Equal(state))
	})

	It("removes only the plugin directory and is idempotent", func() {
		rel := tarRelease("v1.0.4", "x")
		server, _ := serveReleases(rel)
		m := testManager(base, "linux-amd64", server.URL, rel)
		m.Client = server.Client()
		contextFile := filepath.Join(base, "contexts", "default", "config.yaml")
		Expect(os.MkdirAll(filepath.Dir(contextFile), 0o700)).To(Succeed())
		Expect(os.WriteFile(contextFile, []byte("keep"), 0o600)).To(Succeed())
		_, err := m.Install(context.Background(), "bkms", InstallOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(m.Remove("bkms")).To(BeTrue())
		Expect(m.Remove("bkms")).To(BeFalse())
		_, err = os.Stat(filepath.Join(base, "plugins", "bkms"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(os.ReadFile(contextFile)).To(Equal([]byte("keep")))
	})

	It("rejects a corrupted installed.yaml", func() {
		m := testManager(base, "linux-amd64", "https://unused.invalid", tarRelease("v1.0.4", "x"))
		Expect(os.MkdirAll(filepath.Join(base, "plugins"), 0o700)).To(Succeed())
		Expect(os.WriteFile(
			filepath.Join(base, "plugins", "installed.yaml"),
			[]byte("plugins: {bkms: ../../etc}\n"),
			0o600,
		)).To(Succeed())
		_, err := m.InstalledVersion("bkms")
		pErr := expectPluginError(err, CodeStateInvalid)
		statePath := filepath.Join(base, "plugins", installedFileName)
		Expect(pErr.Hint).To(ContainSubstring(statePath))
		Expect(pErr.Hint).To(ContainSubstring("delete"))
		Expect(pErr.Hint).To(ContainSubstring("reinstall"))
	})
})

var _ = Describe("managed plugin edge cases", func() {
	It("bounds downloads and rejects unsafe requests", func() {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("12345"))
		}))
		DeferCleanup(server.Close)

		dst := filepath.Join(GinkgoT().TempDir(), "download")
		err := downloadToFile(context.Background(), server.Client(), server.URL, dst, 4)
		expectPluginError(err, CodeDownloadFailed)

		err = downloadToFile(
			context.Background(),
			server.Client(),
			"http://example.com/archive",
			dst+"-http",
			10,
		)
		expectPluginError(err, CodeDownloadFailed)

		err = downloadToFile(context.Background(), server.Client(), "https://example.com/\x7f", dst+"-url", 10)
		expectPluginError(err, CodeDownloadFailed)
	})

	It("reports destination and request failures during download", func() {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("archive"))
		}))
		DeferCleanup(server.Close)

		dst := filepath.Join(GinkgoT().TempDir(), "existing")
		Expect(os.WriteFile(dst, []byte("existing"), 0o600)).To(Succeed())
		err := downloadToFile(context.Background(), server.Client(), server.URL, dst, 100)
		expectPluginError(err, CodeIOError)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err = downloadToFile(ctx, server.Client(), server.URL, dst+"-canceled", 100)
		expectPluginError(err, CodeDownloadFailed)
	})

	It("enforces redirect policy in the default HTTP client", func() {
		client := newHTTPClient()
		httpsRequest, err := http.NewRequest(http.MethodGet, "https://example.com/archive", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(client.CheckRedirect(httpsRequest, nil)).To(Succeed())
		Expect(client.CheckRedirect(httpsRequest, make([]*http.Request, 10))).To(
			MatchError("too many redirects"),
		)

		httpRequest, err := http.NewRequest(http.MethodGet, "http://example.com/archive", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(client.CheckRedirect(httpRequest, nil)).To(
			MatchError("redirect to non-https url rejected"),
		)
	})

	It("returns actionable errors for missing and malformed managed state", func() {
		base := GinkgoT().TempDir()
		m := testManager(base, "linux-amd64", "https://unused.invalid", tarRelease("v1.0.4", "x"))

		_, err := m.Update(context.Background(), "bkms")
		expectPluginError(err, CodeNotInstalled)
		_, _, err = m.VerifiedExecutable("unknown")
		expectPluginError(err, CodeUnknown)
		_, err = m.Install(context.Background(), "unknown", InstallOptions{})
		expectPluginError(err, CodeUnknown)
		Expect(SystemError(CodeIOError, "failed", "").Error()).To(Equal("plugin_io_error: failed"))

		Expect(os.MkdirAll(filepath.Join(base, "plugins"), 0o700)).To(Succeed())
		Expect(os.WriteFile(
			filepath.Join(base, "plugins", installedFileName),
			[]byte("plugins: ["),
			0o600,
		)).To(Succeed())
		_, err = m.Installed()
		expectPluginError(err, CodeStateInvalid)
	})

	It("accepts an empty state map and reports unreadable state", func() {
		base := GinkgoT().TempDir()
		m := testManager(base, "linux-amd64", "https://unused.invalid", tarRelease("v1.0.4", "x"))
		statePath := filepath.Join(base, "plugins", installedFileName)
		Expect(os.MkdirAll(filepath.Dir(statePath), 0o700)).To(Succeed())
		Expect(os.WriteFile(statePath, []byte("plugins:\n"), 0o600)).To(Succeed())
		Expect(m.Installed()).To(BeEmpty())

		Expect(os.Remove(statePath)).To(Succeed())
		Expect(os.Mkdir(statePath, 0o700)).To(Succeed())
		_, err := m.Installed()
		expectPluginError(err, CodeIOError)
		_, err = m.Update(context.Background(), "bkms")
		expectPluginError(err, CodeIOError)
		_, _, err = m.VerifiedExecutable("bkms")
		expectPluginError(err, CodeIOError)
		_, err = m.Install(context.Background(), "bkms", InstallOptions{})
		expectPluginError(err, CodeIOError)
		_, err = m.Remove("bkms")
		expectPluginError(err, CodeIOError)
	})

	It("validates local archive paths and contents", func() {
		base := GinkgoT().TempDir()
		archive := archiveFixture(
			"tar.gz",
			[]archiveEntry{{Name: "other", Body: "not the executable", Mode: 0o755}},
		)
		rel := releaseFixture{Version: "v1.0.4", Archive: archive, Binary: []byte("expected")}
		m := testManager(base, "linux-amd64", "https://unused.invalid", rel)

		_, err := m.Install(context.Background(), "bkms", InstallOptions{
			FromFile: filepath.Join(base, "missing.tar.gz"),
		})
		pErr := expectPluginError(err, CodeArchiveInvalid)
		Expect(pErr.ExitCode).To(Equal(1))
		Expect(pErr.Message).To(ContainSubstring(filepath.Join(base, "missing.tar.gz")))

		archiveDir := filepath.Join(base, "archive-dir")
		Expect(os.MkdirAll(archiveDir, 0o700)).To(Succeed())
		_, err = m.Install(context.Background(), "bkms", InstallOptions{FromFile: archiveDir})
		pErr = expectPluginError(err, CodeArchiveInvalid)
		Expect(pErr.ExitCode).To(Equal(1))
		Expect(pErr.Message).To(ContainSubstring(archiveDir))

		file := filepath.Join(GinkgoT().TempDir(), "release.tar.gz")
		Expect(os.WriteFile(file, archive, 0o600)).To(Succeed())
		_, err = m.Install(context.Background(), "bkms", InstallOptions{FromFile: file})
		expectPluginError(err, CodeArchiveInvalid)
	})

	It("rejects unsafe store paths and invalid removal names", func() {
		base := GinkgoT().TempDir()
		m := testManager(base, "linux-amd64", "https://unused.invalid", tarRelease("v1.0.4", "x"))

		Expect(m.Remove("../escape")).Error().To(MatchError(ContainSubstring(CodeUnknown)))
		Expect(m.saveInstalled(map[string]string{"bkms": "v1.0.4"})).Error().To(
			MatchError(ContainSubstring(CodeIOError)),
		)
		Expect(m.Remove("legacy-plugin")).To(BeFalse())

		symlinkBase := GinkgoT().TempDir()
		m = testManager(symlinkBase, "linux-amd64", "https://unused.invalid", tarRelease("v1.0.4", "x"))
		outside := GinkgoT().TempDir()
		if err := os.Symlink(outside, filepath.Join(symlinkBase, "plugins")); err != nil {
			Skip("symlinks unavailable: " + err.Error())
		}
		_, err := m.Install(context.Background(), "bkms", InstallOptions{})
		expectPluginError(err, CodeStateInvalid)
	})

	It("reports lock and state replacement I/O errors", func() {
		base := GinkgoT().TempDir()
		m := testManager(base, "linux-amd64", "https://unused.invalid", tarRelease("v1.0.4", "x"))
		pluginsDir := filepath.Join(base, "plugins")
		Expect(os.WriteFile(pluginsDir, []byte("not a directory"), 0o600)).To(Succeed())
		_, err := m.Install(context.Background(), "bkms", InstallOptions{})
		expectPluginError(err, CodeIOError)

		Expect(os.Remove(pluginsDir)).To(Succeed())
		Expect(os.MkdirAll(pluginsDir, 0o700)).To(Succeed())
		Expect(os.Mkdir(filepath.Join(pluginsDir, installedFileName), 0o700)).To(Succeed())
		err = m.saveInstalled(map[string]string{"bkms": "v1.0.4"})
		expectPluginError(err, CodeIOError)
	})

	It("reports invalid process, lock, staging, archive, and managed paths", func() {
		base := GinkgoT().TempDir()
		m := testManager(base, "linux-amd64", "https://unused.invalid", tarRelease("v1.0.4", "x"))

		Expect(processAlive(-1)).To(BeFalse())
		_, err := lockPID(filepath.Join(base, "missing.lock"))
		Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
		expectPluginError(m.cleanStagingDirs(), CodeIOError)
		expectPluginError(validateLocalArchive("invalid\x00archive"), CodeIOError)
		Expect(m.ensureNoSymlinks(filepath.Join(base, "outside"))).To(
			MatchError(ContainSubstring("outside the plugin directory")),
		)
	})

	It("rejects symlinked and non-directory activation paths", func() {
		base := GinkgoT().TempDir()
		m := testManager(base, "linux-amd64", "https://unused.invalid", tarRelease("v1.0.4", "x"))
		r, err := m.Catalog.Resolve("bkms", "v1.0.4", "linux-amd64")
		Expect(err).NotTo(HaveOccurred())
		versionDir := filepath.Join(base, "plugins", "bkms", "v1.0.4")
		Expect(os.MkdirAll(filepath.Dir(versionDir), 0o700)).To(Succeed())
		Expect(os.WriteFile(versionDir, []byte("not a directory"), 0o600)).To(Succeed())
		err = m.activate(r, filepath.Join(base, "missing"))
		expectPluginError(err, CodeIOError)

		Expect(os.Remove(versionDir)).To(Succeed())
		if err := os.Symlink(GinkgoT().TempDir(), versionDir); err != nil {
			Skip("symlinks unavailable: " + err.Error())
		}
		err = m.activate(r, filepath.Join(base, "missing"))
		expectPluginError(err, CodeStateInvalid)
	})

	It("keeps the existing install if it cannot move it to trash", func() {
		base := GinkgoT().TempDir()
		m := testManager(base, "linux-amd64", "https://unused.invalid", tarRelease("v1.0.4", "x"))
		r, err := m.Catalog.Resolve("bkms", "v1.0.4", "linux-amd64")
		Expect(err).NotTo(HaveOccurred())
		final := filepath.Join(base, "plugins", "bkms", "v1.0.4", "linux-amd64")
		Expect(os.MkdirAll(final, 0o700)).To(Succeed())
		trash := final + fmt.Sprintf(".old-%d", os.Getpid())
		Expect(os.MkdirAll(trash, 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(trash, "occupied"), []byte("x"), 0o600)).To(Succeed())

		err = m.activate(r, filepath.Join(base, "missing"))
		expectPluginError(err, CodeIOError)
		_, statErr := os.Stat(final)
		Expect(statErr).NotTo(HaveOccurred())
	})

	It("rejects a symlinked directory for a catalog-removed plugin", func() {
		base := GinkgoT().TempDir()
		m := testManager(base, "linux-amd64", "https://unused.invalid", tarRelease("v1.0.4", "x"))
		pluginsDir := filepath.Join(base, "plugins")
		Expect(os.MkdirAll(pluginsDir, 0o700)).To(Succeed())
		Expect(os.WriteFile(
			filepath.Join(pluginsDir, installedFileName),
			[]byte("plugins: {legacy-plugin: v1.0.4}\n"),
			0o600,
		)).To(Succeed())
		if err := os.Symlink(GinkgoT().TempDir(), filepath.Join(pluginsDir, "legacy-plugin")); err != nil {
			Skip("symlinks unavailable: " + err.Error())
		}

		_, err := m.Remove("legacy-plugin")
		expectPluginError(err, CodeStateInvalid)
	})

	It("restores an existing install when activation rename fails", func() {
		base := GinkgoT().TempDir()
		m := testManager(base, "linux-amd64", "https://unused.invalid", tarRelease("v1.0.4", "x"))
		r, err := m.Catalog.Resolve("bkms", "v1.0.4", "linux-amd64")
		Expect(err).NotTo(HaveOccurred())
		final := filepath.Join(base, "plugins", "bkms", "v1.0.4", "linux-amd64")
		Expect(os.MkdirAll(final, 0o700)).To(Succeed())
		marker := filepath.Join(final, "marker")
		Expect(os.WriteFile(marker, []byte("old"), 0o600)).To(Succeed())

		err = m.activate(r, filepath.Join(base, "plugins", "missing-staging"))
		expectPluginError(err, CodeIOError)
		Expect(os.ReadFile(marker)).To(Equal([]byte("old")))
		leftovers, globErr := filepath.Glob(final + ".old-*")
		Expect(globErr).NotTo(HaveOccurred())
		Expect(leftovers).To(BeEmpty())
	})
})
