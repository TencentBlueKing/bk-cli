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
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	json "github.com/goccy/go-json"
)

var githubHTTPClient = &http.Client{Timeout: 10 * time.Second}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// GitHubSource checks for updates from GitHub Releases published by GoReleaser.
type GitHubSource struct {
	Owner   string
	Repo    string
	APIBase string
}

func (s *GitHubSource) apiBase() string {
	if strings.TrimSpace(s.APIBase) != "" {
		return strings.TrimRight(s.APIBase, "/")
	}
	return "https://api.github.com"
}

// CheckLatest fetches the latest GitHub release and returns the platform-specific asset URL.
func (s *GitHubSource) CheckLatest(ctx context.Context) (*ReleaseInfo, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", s.apiBase(), s.Owner, s.Repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create update request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := githubHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("version check returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release info: %w", err)
	}

	if release.TagName == "" {
		return nil, fmt.Errorf("latest release has no tag name")
	}

	assetName := expectedReleaseAssetName(release.TagName, runtime.GOOS, runtime.GOARCH)
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			if asset.BrowserDownloadURL == "" {
				return nil, fmt.Errorf("release asset %q has no download URL", assetName)
			}
			return &ReleaseInfo{
				Version:     strings.TrimPrefix(release.TagName, "v"),
				DownloadURL: asset.BrowserDownloadURL,
			}, nil
		}
	}

	return nil, fmt.Errorf("release asset %q not found in latest release", assetName)
}

func expectedReleaseAssetName(version, goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("bk-cli_%s_%s_%s.%s", version, goos, goarch, ext)
}
