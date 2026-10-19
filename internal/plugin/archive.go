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
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func archiveInvalid(format string, args ...any) *Error {
	return SystemError(CodeArchiveInvalid, fmt.Sprintf(format, args...), "")
}

func entryMatches(name, executable string) bool {
	return strings.TrimPrefix(name, "./") == executable
}

// ExtractExecutable copies the single declared top-level executable entry to dst.
// Every other entry is ignored, so its path is never used on disk.
func ExtractExecutable(archivePath, format, executable string, dst io.Writer) error {
	switch format {
	case "tar.gz":
		return extractFromTarGz(archivePath, executable, dst)
	case "zip":
		return extractFromZip(archivePath, executable, dst)
	default:
		return archiveInvalid("unsupported archive format %q", format)
	}
}

func extractFromTarGz(archivePath, executable string, dst io.Writer) error {
	// #nosec G304 -- archivePath is a staged or user-selected archive, only read.
	f, err := os.Open(archivePath)
	if err != nil {
		return SystemError(CodeIOError, err.Error(), "")
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return archiveInvalid("invalid gzip archive: %v", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	found := false
	entryCount := 0
	for {
		h, nextErr := tr.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return archiveInvalid("invalid tar archive: %v", nextErr)
		}
		entryCount++
		if entryCount > maxArchiveEntries {
			return archiveInvalid("archive has too many entries")
		}
		if !entryMatches(h.Name, executable) {
			continue
		}
		if found {
			return archiveInvalid("archive contains %q more than once", executable)
		}
		found = true
		if h.Typeflag != tar.TypeReg {
			return archiveInvalid("%q is not a regular file", executable)
		}
		if h.Size > maxExecutableBytes {
			return archiveInvalid("%q exceeds size limit", executable)
		}
		n, copyErr := io.Copy(dst, io.LimitReader(tr, maxExecutableBytes+1))
		if copyErr != nil {
			return archiveInvalid("read %q: %v", executable, copyErr)
		}
		if n > maxExecutableBytes {
			return archiveInvalid("%q exceeds size limit", executable)
		}
	}
	if !found {
		return archiveInvalid("archive does not contain top-level %q", executable)
	}
	return nil
}

func extractFromZip(archivePath, executable string, dst io.Writer) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return archiveInvalid("invalid zip archive: %v", err)
	}
	defer func() { _ = zr.Close() }()
	if len(zr.File) > maxArchiveEntries {
		return archiveInvalid("archive has too many entries")
	}

	var match *zip.File
	for _, zf := range zr.File {
		if !entryMatches(zf.Name, executable) {
			continue
		}
		if match != nil {
			return archiveInvalid("archive contains %q more than once", executable)
		}
		match = zf
	}
	if match == nil {
		return archiveInvalid("archive does not contain top-level %q", executable)
	}
	if !match.Mode().IsRegular() {
		return archiveInvalid("%q is not a regular file", executable)
	}
	if match.UncompressedSize64 > uint64(maxExecutableBytes) {
		return archiveInvalid("%q exceeds size limit", executable)
	}
	rc, err := match.Open()
	if err != nil {
		return archiveInvalid("open %q: %v", executable, err)
	}
	defer func() { _ = rc.Close() }()
	n, copyErr := io.Copy(dst, io.LimitReader(rc, maxExecutableBytes+1))
	if copyErr != nil {
		return archiveInvalid("read %q: %v", executable, copyErr)
	}
	if n > maxExecutableBytes {
		return archiveInvalid("%q exceeds size limit", executable)
	}
	return nil
}

// FileSHA256 returns the lowercase hex SHA-256 of a regular, non-symlink file.
func FileSHA256(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	// #nosec G304 -- path was checked to be a regular file.
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
