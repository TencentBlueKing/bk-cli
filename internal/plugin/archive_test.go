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
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ExtractExecutable", func() {
	write := func(data []byte) string {
		p := filepath.Join(GinkgoT().TempDir(), "a")
		Expect(os.WriteFile(p, data, 0o600)).To(Succeed())
		return p
	}

	DescribeTable(
		"extracts only the declared entry",
		func(format string) {
			p := write(archiveFixture(format, []archiveEntry{
				{Name: "../outside", Body: "evil", Mode: 0o644},
				{Name: "bkms-cli", Body: "tool", Mode: 0o755},
			}))
			var out bytes.Buffer
			Expect(ExtractExecutable(p, format, "bkms-cli", &out)).To(Succeed())
			Expect(out.String()).To(Equal("tool"))
			_, err := os.Stat(filepath.Join(filepath.Dir(p), "..", "outside"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		},
		Entry("tar.gz", "tar.gz"),
		Entry("zip", "zip"),
	)

	It("accepts a ./ prefix", func() {
		p := write(archiveFixture(
			"tar.gz",
			[]archiveEntry{{Name: "./bkms-cli", Body: "tool", Mode: 0o755}},
		))
		var out bytes.Buffer
		Expect(ExtractExecutable(p, "tar.gz", "bkms-cli", &out)).To(Succeed())
	})

	DescribeTable(
		"rejects unsafe or missing entries",
		func(format string, entries []archiveEntry) {
			var out bytes.Buffer
			err := ExtractExecutable(write(archiveFixture(format, entries)), format, "bkms-cli", &out)
			expectPluginError(err, CodeArchiveInvalid)
		},
		Entry("missing", "tar.gz", []archiveEntry{{Name: "other", Body: "x", Mode: 0o755}}),
		Entry("zip missing", "zip", []archiveEntry{{Name: "other", Body: "x", Mode: 0o755}}),
		Entry("nested only", "tar.gz", []archiveEntry{{Name: "dir/bkms-cli", Body: "x", Mode: 0o755}}),
		Entry("duplicate", "tar.gz", []archiveEntry{
			{Name: "bkms-cli", Body: "a", Mode: 0o755},
			{Name: "bkms-cli", Body: "b", Mode: 0o755},
		}),
		Entry("tar symlink", "tar.gz", []archiveEntry{{Name: "bkms-cli", Symlink: "/bin/sh"}}),
		Entry("tar hardlink", "tar.gz", []archiveEntry{{Name: "bkms-cli", Hardlink: "other"}}),
		Entry("zip symlink", "zip", []archiveEntry{{Name: "bkms-cli", Symlink: "/bin/sh"}}),
	)

	It("rejects truncated archives", func() {
		data := archiveFixture(
			"tar.gz",
			[]archiveEntry{{Name: "bkms-cli", Body: "tool", Mode: 0o755}},
		)
		var out bytes.Buffer
		err := ExtractExecutable(write(data[:len(data)/2]), "tar.gz", "bkms-cli", &out)
		expectPluginError(err, CodeArchiveInvalid)
	})

	It("rejects an unsupported archive format", func() {
		var out bytes.Buffer
		err := ExtractExecutable(write([]byte("archive")), "rar", "bkms-cli", &out)
		expectPluginError(err, CodeArchiveInvalid)
	})

	It("rejects an invalid zip archive", func() {
		var out bytes.Buffer
		err := ExtractExecutable(write([]byte("not a zip")), "zip", "bkms-cli", &out)
		expectPluginError(err, CodeArchiveInvalid)
	})

	It("rejects duplicate zip entries", func() {
		data := archiveFixture("zip", []archiveEntry{
			{Name: "bkms-cli", Body: "a", Mode: 0o755},
			{Name: "./bkms-cli", Body: "b", Mode: 0o755},
		})
		var out bytes.Buffer
		err := ExtractExecutable(write(data), "zip", "bkms-cli", &out)
		expectPluginError(err, CodeArchiveInvalid)
	})

	It("rejects a declared oversized executable without reading its body", func() {
		var tarData bytes.Buffer
		tw := tar.NewWriter(&tarData)
		Expect(tw.WriteHeader(&tar.Header{
			Name: "bkms-cli", Mode: 0o755, Size: maxExecutableBytes + 1, Typeflag: tar.TypeReg,
		})).To(Succeed())

		var archive bytes.Buffer
		gz := gzip.NewWriter(&archive)
		_, err := gz.Write(tarData.Bytes())
		Expect(err).NotTo(HaveOccurred())
		Expect(gz.Close()).To(Succeed())

		var out bytes.Buffer
		err = ExtractExecutable(write(archive.Bytes()), "tar.gz", "bkms-cli", &out)
		expectPluginError(err, CodeArchiveInvalid)
		Expect(out.Len()).To(BeZero())
	})

	It("rejects a zip entry whose directory declares an oversized executable", func() {
		data := archiveFixture(
			"zip",
			[]archiveEntry{{Name: "bkms-cli", Body: "tool", Mode: 0o755}},
		)
		const centralDirectorySignature = "PK\x01\x02"
		offset := bytes.Index(data, []byte(centralDirectorySignature))
		Expect(offset).To(BeNumerically(">=", 0))
		binary.LittleEndian.PutUint32(data[offset+24:offset+28], uint32(maxExecutableBytes+1))

		var out bytes.Buffer
		err := ExtractExecutable(write(data), "zip", "bkms-cli", &out)
		expectPluginError(err, CodeArchiveInvalid)
		Expect(out.Len()).To(BeZero())
	})

	DescribeTable(
		"rejects archives with too many entries",
		func(format string) {
			entries := make([]archiveEntry, maxArchiveEntries+1)
			for i := range entries {
				entries[i] = archiveEntry{Name: "entry-" + strconv.Itoa(i), Mode: 0o644}
			}
			var out bytes.Buffer
			err := ExtractExecutable(write(archiveFixture(format, entries)), format, "bkms-cli", &out)
			expectPluginError(err, CodeArchiveInvalid)
		},
		Entry("tar.gz", "tar.gz"),
		Entry("zip", "zip"),
	)
})

var _ = Describe("FileSHA256", func() {
	It("rejects non-regular files and missing paths", func() {
		dir := GinkgoT().TempDir()
		_, err := FileSHA256(dir)
		Expect(err).To(MatchError(ContainSubstring("not a regular file")))

		_, err = FileSHA256(filepath.Join(dir, "missing"))
		Expect(err).To(MatchError(os.ErrNotExist))
	})
})
