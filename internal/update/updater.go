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

// Package update checks a remote repository for newer releases and replaces the current binary.
package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const npmPackage = "@blueking/bk-cli"

var (
	downloadHTTPClient = &http.Client{Timeout: 5 * time.Minute}
	executablePath     = os.Executable
	execCommand        = exec.CommandContext
)

// ReleaseInfo holds the version and download URL for an update.
type ReleaseInfo struct {
	Version     string
	DownloadURL string
}

// Source abstracts how update information is retrieved.
// Implementations handle version checking and download URL construction
// for different backends (GitHub Releases, etc.).
type Source interface {
	// CheckLatest returns the latest available release info,
	// including the version and the platform-specific download URL.
	CheckLatest(ctx context.Context) (*ReleaseInfo, error)
}

// IsNewer returns true if the remote version is newer than current.
// Uses golang.org/x/mod/semver for correct semantic version comparison.
func IsNewer(current, remote string) bool {
	// Ensure canonical "v" prefix for semver package
	if !strings.HasPrefix(current, "v") {
		current = "v" + current
	}
	if !strings.HasPrefix(remote, "v") {
		remote = "v" + remote
	}

	// semver.Compare returns -1, 0, or +1
	// If either is not valid semver, Compare returns 0; fall back to inequality check
	if !semver.IsValid(current) || !semver.IsValid(remote) {
		return remote != current
	}
	return semver.Compare(remote, current) > 0
}

// IsNPMInstall checks whether the current executable appears to have been installed via npm.
func IsNPMInstall() bool {
	execPath, err := executablePath()
	if err != nil {
		return false
	}

	// Normalize path separators for consistent matching
	normalized := filepath.ToSlash(execPath)
	lower := strings.ToLower(normalized)

	return strings.Contains(lower, "node_modules") ||
		strings.Contains(lower, "/lib/node_modules/")
}

// RunNPMUpdate runs `npm install -g @blueking/bk-cli` to update the package from the public npm registry.
func RunNPMUpdate(ctx context.Context) ([]byte, error) {
	cmd := execCommand(ctx, "npm", "install", "-g", npmPackage)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("npm update failed: %w", err)
	}
	return out, nil
}

// DownloadAndReplace downloads a tar.gz archive from the given URL,
// extracts the bk-cli binary, and replaces the current executable.
func DownloadAndReplace(downloadURL string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := downloadHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// Get current executable path
	execPath, err := executablePath()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	execInfo, err := os.Stat(execPath)
	if err != nil {
		return fmt.Errorf("cannot stat executable: %w", err)
	}
	execMode := execInfo.Mode().Perm()

	// Extract binary from tar.gz
	binaryData, err := extractBinaryFromTarGz(resp.Body)
	if err != nil {
		return err
	}

	// Write to temp file in same directory as executable to ensure same filesystem
	tmpFile, err := os.CreateTemp(filepath.Dir(execPath), "bk-cli-update-*")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	defer func() {
		_ = os.Remove(tmpFile.Name())
	}()

	if _, err := tmpFile.Write(binaryData); err != nil {
		if closeErr := tmpFile.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			return fmt.Errorf("write failed: %w", errors.Join(err, closeErr))
		}
		return fmt.Errorf("write failed: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Replace
	if err := os.Rename(tmpFile.Name(), execPath); err != nil {
		return fmt.Errorf("replace failed: %w", err)
	}
	if err := os.Chmod(execPath, execMode); err != nil {
		return fmt.Errorf("restore executable permissions failed: %w", err)
	}

	return nil
}

// extractBinaryFromTarGz reads a tar.gz stream and returns the content of the bk-cli binary.
func extractBinaryFromTarGz(r io.Reader) ([]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to open gzip stream: %w", err)
	}
	defer func() {
		_ = gz.Close()
	}()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar archive: %w", err)
		}

		// Match the bk-cli binary (may be at root or inside a directory)
		name := filepath.Base(header.Name)
		if name == "bk-cli" && header.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("failed to extract binary: %w", err)
			}
			return data, nil
		}
	}

	return nil, fmt.Errorf("bk-cli binary not found in archive")
}
