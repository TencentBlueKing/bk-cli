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
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// EntryOptions configures BuildReleaseEntry for a single plugin release.
type EntryOptions struct {
	ReleaseBaseURL string   // e.g. https://github.com/o/r/releases/download/bkms-cli/v1.0.4
	AssetTemplate  string   // e.g. bkms-cli_{version}_{os}_{arch} without extension
	Executable     string   // e.g. bkms-cli; Windows adds .exe automatically
	Version        string   // e.g. v1.0.4
	Auth           string   // none | shared
	Platforms      []string // empty uses SupportedPlatforms
}

// BuildReleaseEntry downloads each platform archive of a release, checks it against the release
// checksums.txt, extracts the executable and returns a catalog Release entry. Maintainer use only.
func BuildReleaseEntry(ctx context.Context, client *http.Client, opts EntryOptions) (Release, error) {
	if opts.Auth != "none" && opts.Auth != "shared" {
		return Release{}, UserError(CodeCatalogInvalid, "auth must be none or shared", "")
	}
	if !isSafeBasename(opts.Executable) {
		return Release{}, UserError(CodeCatalogInvalid, "executable must be a safe basename", "")
	}

	platforms := opts.Platforms
	if len(platforms) == 0 {
		platforms = SupportedPlatforms
	}
	for _, platform := range platforms {
		if err := validatePlatform(platform); err != nil {
			return Release{}, err
		}
		if _, err := archiveNameForPlatform(opts, platform); err != nil {
			return Release{}, err
		}
	}

	if client == nil {
		client = newHTTPClient()
	}

	dir, err := os.MkdirTemp("", "bk-cli-catalog-entry-*")
	if err != nil {
		return Release{}, SystemError(CodeIOError, err.Error(), "")
	}
	defer func() { _ = os.RemoveAll(dir) }()

	base := strings.TrimSuffix(opts.ReleaseBaseURL, "/")
	sumsPath := filepath.Join(dir, "checksums.txt")
	if err := downloadToFile(ctx, client, base+"/checksums.txt", sumsPath, 1<<20); err != nil {
		return Release{}, err
	}
	sums, err := parseChecksums(sumsPath)
	if err != nil {
		return Release{}, err
	}

	rel := Release{Status: "allowed", Auth: opts.Auth, Platforms: map[string]Asset{}}
	for _, platform := range platforms {
		name, err := archiveNameForPlatform(opts, platform)
		if err != nil {
			return Release{}, err
		}
		expected, ok := sums[name]
		if !ok {
			return Release{}, SystemError(
				CodeDownloadFailed,
				"checksums.txt has no entry for "+name,
				"",
			)
		}
		archive := filepath.Join(dir, name)
		if err := downloadToFile(ctx, client, base+"/"+name, archive, maxArchiveBytes); err != nil {
			return Release{}, err
		}
		if sum, err := FileSHA256(archive); err != nil || sum != expected {
			return Release{}, SystemError(
				CodeDigestMismatch,
				name+" does not match checksums.txt",
				"",
			)
		}
		exe := opts.Executable
		if strings.HasPrefix(platform, "windows-") {
			exe += ".exe"
		}
		exePath := filepath.Join(dir, platform+"-"+exe)
		f, err := os.Create(exePath) // #nosec G304 -- temp file inside a private temp directory.
		if err != nil {
			return Release{}, SystemError(CodeIOError, err.Error(), "")
		}
		extractErr := ExtractExecutable(archive, ArchiveFormat(platform), exe, f)
		closeErr := f.Close()
		if extractErr != nil {
			return Release{}, extractErr
		}
		if closeErr != nil {
			return Release{}, SystemError(CodeIOError, closeErr.Error(), "")
		}
		exeSum, err := FileSHA256(exePath)
		if err != nil {
			return Release{}, SystemError(CodeIOError, err.Error(), "")
		}
		rel.Platforms[platform] = Asset{
			URL:              base + "/" + name,
			ArchiveSHA256:    expected,
			Executable:       exe,
			ExecutableSHA256: exeSum,
		}
	}
	return rel, nil
}

func archiveNameForPlatform(opts EntryOptions, platform string) (string, error) {
	goos, goarch, _ := strings.Cut(platform, "-")
	name := strings.NewReplacer(
		"{version}", strings.TrimPrefix(opts.Version, "v"),
		"{os}", goos,
		"{arch}", goarch,
	).Replace(opts.AssetTemplate) + "." + ArchiveFormat(platform)
	if !isSafeBasename(name) {
		return "", UserError(
			CodeCatalogInvalid,
			fmt.Sprintf("archive name must be a safe basename: %q", name),
			"",
		)
	}
	return name, nil
}

func isSafeBasename(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.Contains(name, "/") || strings.Contains(name, `\`) {
		return false
	}
	return filepath.Base(name) == name
}

func validatePlatform(platform string) error {
	goos, goarch, found := strings.Cut(platform, "-")
	if !found || goos == "" || goarch == "" || strings.Contains(goarch, "-") {
		return UserError(CodeCatalogInvalid, fmt.Sprintf("unsupported platform %q", platform), "")
	}
	if !slices.Contains(SupportedPlatforms, platform) {
		return UserError(CodeCatalogInvalid, fmt.Sprintf("unsupported platform %q", platform), "")
	}
	return nil
}

func parseChecksums(path string) (map[string]string, error) {
	f, err := os.Open(path) // #nosec G304 -- temp file inside a private temp directory.
	if err != nil {
		return nil, SystemError(CodeIOError, err.Error(), "")
	}
	defer func() { _ = f.Close() }()
	sums := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && sha256Pattern.MatchString(fields[0]) {
			sums[strings.TrimPrefix(fields[1], "*")] = fields[0]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, SystemError(CodeIOError, err.Error(), "")
	}
	return sums, nil
}
