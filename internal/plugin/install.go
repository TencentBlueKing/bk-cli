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
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

// InstallOptions selects the catalog version and optional local release archive.
type InstallOptions struct {
	Version  string
	FromFile string
}

// InstallResult describes the active managed plugin after an install or update.
type InstallResult struct {
	Plugin   string `json:"plugin"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Path     string `json:"path"`
	Changed  bool   `json:"changed"`
}

// Install installs an approved version (recommended when opts.Version is empty) and makes it current.
func (m *Manager) Install(ctx context.Context, name string, opts InstallOptions) (InstallResult, error) {
	r, err := m.Catalog.Resolve(name, opts.Version, m.Platform)
	if err != nil {
		return InstallResult{}, err
	}
	if err := validateLocalArchive(opts.FromFile); err != nil {
		return InstallResult{}, err
	}
	unlock, err := m.lock()
	if err != nil {
		return InstallResult{}, err
	}
	defer unlock()

	installed, err := m.Installed()
	if err != nil {
		return InstallResult{}, err
	}
	return m.installLocked(ctx, r, opts, installed)
}

func validateLocalArchive(path string) error {
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return UserError(
			CodeArchiveInvalid,
			fmt.Sprintf("local plugin archive %q does not exist", path),
			"Provide an existing official release archive",
		)
	}
	if err != nil {
		return SystemError(CodeIOError, fmt.Sprintf("inspect local plugin archive %q: %v", path, err), "")
	}
	if !info.Mode().IsRegular() {
		return UserError(
			CodeArchiveInvalid,
			fmt.Sprintf("local plugin archive %q is not a regular file", path),
			"Provide an official release archive file, not a directory",
		)
	}
	return nil
}

func (m *Manager) installLocked(
	ctx context.Context,
	r Resolved,
	opts InstallOptions,
	installed map[string]string,
) (InstallResult, error) {
	target := m.executablePath(r)
	result := InstallResult{
		Plugin: r.Name, Version: r.Version, Platform: r.Platform, Path: target,
	}
	if installed[r.Name] == r.Version && m.verify(r) == nil {
		return result, nil
	}

	staging, err := os.MkdirTemp(m.pluginsDir(), ".staging-*")
	if err != nil {
		return InstallResult{}, SystemError(CodeIOError, err.Error(), "")
	}
	defer func() { _ = os.RemoveAll(staging) }()

	archive := opts.FromFile
	if archive == "" {
		archive = filepath.Join(staging, "archive")
		if err := downloadToFile(ctx, m.Client, r.Asset.URL, archive, maxArchiveBytes); err != nil {
			return InstallResult{}, err
		}
	}
	if sum, err := FileSHA256(archive); err != nil {
		return InstallResult{}, SystemError(CodeIOError, err.Error(), "")
	} else if sum != r.Asset.ArchiveSHA256 {
		return InstallResult{}, SystemError(
			CodeDigestMismatch,
			"archive checksum does not match the bk-cli catalog",
			"Use the official release archive for this exact version and platform",
		)
	}

	platformDir := filepath.Join(staging, r.Platform)
	if err := os.Mkdir(platformDir, 0o700); err != nil {
		return InstallResult{}, SystemError(CodeIOError, err.Error(), "")
	}
	exePath := filepath.Join(platformDir, r.Asset.Executable)
	if err := writeExecutable(archive, r, exePath); err != nil {
		return InstallResult{}, err
	}
	if sum, err := FileSHA256(exePath); err != nil || sum != r.Asset.ExecutableSHA256 {
		return InstallResult{}, SystemError(
			CodeDigestMismatch,
			"executable checksum does not match the bk-cli catalog",
			"",
		)
	}

	if err := m.activate(r, platformDir); err != nil {
		return InstallResult{}, err
	}
	previous := installed[r.Name]
	installed[r.Name] = r.Version
	if err := m.saveInstalled(installed); err != nil {
		return InstallResult{}, err
	}
	if previous != "" && previous != r.Version {
		_ = os.RemoveAll(filepath.Join(m.pluginsDir(), r.Name, previous))
	}
	result.Changed = true
	return result, nil
}

func writeExecutable(archive string, r Resolved, dst string) error {
	// #nosec G302 G304 -- plugin executable is created inside the private staging directory.
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return SystemError(CodeIOError, err.Error(), "")
	}
	extractErr := ExtractExecutable(archive, ArchiveFormat(r.Platform), r.Asset.Executable, f)
	closeErr := f.Close()
	if extractErr != nil {
		return extractErr
	}
	if closeErr != nil {
		return SystemError(CodeIOError, closeErr.Error(), "")
	}
	return nil
}

// activate moves the verified staging platform directory into its final location.
func (m *Manager) activate(r Resolved, platformDir string) error {
	versionDir := filepath.Join(m.pluginsDir(), r.Name, r.Version)
	if err := m.ensureNoSymlinks(versionDir); err != nil {
		return SystemError(CodeStateInvalid, err.Error(), "")
	}
	if err := os.MkdirAll(versionDir, 0o700); err != nil {
		return SystemError(CodeIOError, err.Error(), "")
	}
	if err := m.ensureNoSymlinks(versionDir); err != nil {
		return SystemError(CodeStateInvalid, err.Error(), "")
	}

	final := filepath.Join(versionDir, r.Platform)
	trash := ""
	if _, err := os.Lstat(final); err == nil {
		trash = final + fmt.Sprintf(".old-%d", os.Getpid())
		if err := os.Rename(final, trash); err != nil {
			return SystemError(CodeIOError, err.Error(), "")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return SystemError(CodeIOError, err.Error(), "")
	}
	if err := os.Rename(platformDir, final); err != nil {
		if trash != "" {
			if restoreErr := os.Rename(trash, final); restoreErr != nil {
				return SystemError(
					CodeIOError,
					fmt.Sprintf(
						"activate plugin: %v; restore previous install: %v",
						err,
						restoreErr,
					),
					"",
				)
			}
		}
		return SystemError(CodeIOError, err.Error(), "")
	}
	if trash != "" {
		_ = os.RemoveAll(trash)
	}
	return nil
}

func (m *Manager) verify(r Resolved) error {
	path := m.executablePath(r)
	if err := m.ensureNoSymlinks(path); err != nil {
		return SystemError(CodeDigestMismatch, "installed plugin path contains a symlink", "")
	}
	sum, err := FileSHA256(path)
	if err != nil || sum != r.Asset.ExecutableSHA256 {
		return SystemError(
			CodeDigestMismatch,
			fmt.Sprintf("installed %s %s does not match the bk-cli catalog", r.Name, r.Version),
			fmt.Sprintf("Run: bk-cli plugin install %s --version %s", r.Name, r.Version),
		)
	}
	return nil
}

// Update installs the catalog recommended version for an installed plugin.
func (m *Manager) Update(ctx context.Context, name string) (InstallResult, error) {
	unlock, err := m.lock()
	if err != nil {
		return InstallResult{}, err
	}
	defer unlock()

	installed, err := m.Installed()
	if err != nil {
		return InstallResult{}, err
	}
	if installed[name] == "" {
		return InstallResult{}, UserError(
			CodeNotInstalled,
			fmt.Sprintf("plugin %q is not installed", name),
			"Run: bk-cli plugin install "+name,
		)
	}
	r, err := m.Catalog.Resolve(name, "", m.Platform)
	if err != nil {
		return InstallResult{}, err
	}
	return m.installLocked(ctx, r, InstallOptions{}, installed)
}

// Remove deletes an installed plugin. Names no longer in the catalog can still be removed.
func (m *Manager) Remove(name string) (bool, error) {
	if _, inCatalog := m.Catalog.Plugins[name]; !inCatalog {
		if err := validate.ValidateContextName(name); err != nil {
			return false, UserError(CodeUnknown, fmt.Sprintf("invalid plugin name %q", name), "")
		}
	}
	unlock, err := m.lock()
	if err != nil {
		return false, err
	}
	defer unlock()

	installed, err := m.Installed()
	if err != nil {
		return false, err
	}
	if _, ok := installed[name]; !ok {
		return false, nil
	}
	dir := filepath.Join(m.pluginsDir(), name)
	if err := m.ensureNoSymlinks(dir); err != nil {
		return false, SystemError(CodeStateInvalid, err.Error(), "")
	}
	delete(installed, name)
	if err := m.saveInstalled(installed); err != nil {
		return false, err
	}
	if err := os.RemoveAll(dir); err != nil {
		return false, SystemError(CodeIOError, err.Error(), "")
	}
	return true, nil
}

// VerifiedExecutable returns the catalog entry and absolute path of the installed plugin after
// re-checking catalog authorization and the executable digest. It never consults PATH.
func (m *Manager) VerifiedExecutable(name string) (Resolved, string, error) {
	if _, ok := m.Catalog.Plugins[name]; !ok {
		return Resolved{}, "", UserError(
			CodeUnknown,
			fmt.Sprintf("plugin %q is not in the bk-cli catalog", name),
			"",
		)
	}
	version, err := m.InstalledVersion(name)
	if err != nil {
		return Resolved{}, "", err
	}
	if version == "" {
		return Resolved{}, "", UserError(
			CodeNotInstalled,
			fmt.Sprintf("plugin %q is not installed", name),
			"Run: bk-cli plugin install "+name,
		)
	}
	r, err := m.Catalog.Resolve(name, version, m.Platform)
	if err != nil {
		var pErr *Error
		if errors.As(err, &pErr) && pErr.Code == CodeVersionNotAllowed {
			pErr.Hint = "Run: bk-cli plugin update " + name
		}
		return Resolved{}, "", err
	}
	if err := m.verify(r); err != nil {
		return Resolved{}, "", err
	}
	abs, err := filepath.Abs(m.executablePath(r))
	if err != nil {
		return Resolved{}, "", SystemError(CodeIOError, err.Error(), "")
	}
	return r, abs, nil
}
