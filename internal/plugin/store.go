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
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
	"golang.org/x/mod/semver"

	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

const installedFileName = "installed.yaml"

// Manager manages catalog-authorized plugins in one configuration directory.
type Manager struct {
	Catalog  *Catalog
	BaseDir  string // config.BaseDirectory()
	Platform string // goos + "-" + goarch
	Client   *http.Client
}

type installedFile struct {
	Plugins map[string]string `yaml:"plugins"`
}

// NewManager creates a plugin manager for one platform and config base directory.
func NewManager(catalog *Catalog, baseDir, goos, goarch string) *Manager {
	return &Manager{
		Catalog:  catalog,
		BaseDir:  baseDir,
		Platform: goos + "-" + goarch,
		Client:   newHTTPClient(),
	}
}

func (m *Manager) pluginsDir() string {
	return filepath.Join(m.BaseDir, "plugins")
}

func (m *Manager) executablePath(r Resolved) string {
	return filepath.Join(m.pluginsDir(), r.Name, r.Version, r.Platform, r.Asset.Executable)
}

// ensureNoSymlinks rejects symlinks on the path from the plugins directory down to target.
func (m *Manager) ensureNoSymlinks(target string) error {
	rel, err := filepath.Rel(m.pluginsDir(), target)
	outside := rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
	if err != nil || outside {
		return fmt.Errorf("%s is outside the plugin directory", target)
	}
	current := m.pluginsDir()
	parts := []string{""}
	if rel != "." {
		parts = append(parts, strings.Split(rel, string(filepath.Separator))...)
	}
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, lstatErr := os.Lstat(current)
		if errors.Is(lstatErr, os.ErrNotExist) {
			return nil
		}
		if lstatErr != nil {
			return lstatErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink", current)
		}
	}
	return nil
}

// Installed returns the installed plugin versions. A missing file means nothing is installed.
func (m *Manager) Installed() (map[string]string, error) {
	path := filepath.Join(m.pluginsDir(), installedFileName)
	// #nosec G304 -- fixed file under the plugin directory.
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, SystemError(CodeIOError, err.Error(), "")
	}
	var f installedFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, SystemError(
			CodeStateInvalid,
			fmt.Sprintf("invalid %s: %v", path, err),
			invalidStateHint(path),
		)
	}
	for name, version := range f.Plugins {
		if validate.ValidateContextName(name) != nil ||
			!semver.IsValid(version) ||
			semver.Canonical(version) != version {
			return nil, SystemError(
				CodeStateInvalid,
				"invalid entry in "+path,
				invalidStateHint(path),
			)
		}
	}
	if f.Plugins == nil {
		f.Plugins = map[string]string{}
	}
	return f.Plugins, nil
}

func invalidStateHint(path string) string {
	return fmt.Sprintf(
		"To recover, delete %s and reinstall plugins with: bk-cli plugin install <name>",
		path,
	)
}

// InstalledVersion returns the installed version of name, or "" if not installed.
func (m *Manager) InstalledVersion(name string) (string, error) {
	all, err := m.Installed()
	if err != nil {
		return "", err
	}
	return all[name], nil
}

func (m *Manager) saveInstalled(plugins map[string]string) error {
	data, err := yaml.Marshal(installedFile{Plugins: plugins})
	if err != nil {
		return SystemError(CodeIOError, err.Error(), "")
	}
	tmp, err := os.CreateTemp(m.pluginsDir(), ".installed-*")
	if err != nil {
		return SystemError(CodeIOError, err.Error(), "")
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return SystemError(CodeIOError, err.Error(), "")
	}
	if err := tmp.Close(); err != nil {
		return SystemError(CodeIOError, err.Error(), "")
	}
	if err := os.Rename(tmp.Name(), filepath.Join(m.pluginsDir(), installedFileName)); err != nil {
		return SystemError(CodeIOError, err.Error(), "")
	}
	return nil
}

// lock takes the plugin directory lock, replacing a lock held by a process that is no longer alive.
func (m *Manager) lock() (func(), error) {
	if err := os.MkdirAll(m.pluginsDir(), 0o700); err != nil {
		return nil, SystemError(CodeIOError, err.Error(), "")
	}
	if err := m.ensureNoSymlinks(m.pluginsDir()); err != nil {
		return nil, SystemError(CodeStateInvalid, err.Error(), "")
	}
	path := filepath.Join(m.pluginsDir(), ".lock")
	for {
		// #nosec G304 -- fixed lock file under the plugin directory.
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if errors.Is(err, os.ErrExist) {
			pid, readErr := lockPID(path)
			if readErr != nil || processAlive(pid) {
				return nil, SystemError(
					CodeBusy,
					"another plugin operation is in progress",
					"If no bk-cli plugin command is running, delete "+path,
				)
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, SystemError(CodeIOError, err.Error(), "")
			}
			continue
		}
		if err != nil {
			return nil, SystemError(CodeIOError, err.Error(), "")
		}
		if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return nil, SystemError(CodeIOError, err.Error(), "")
		}
		if err := f.Close(); err != nil {
			_ = os.Remove(path)
			return nil, SystemError(CodeIOError, err.Error(), "")
		}
		if err := m.cleanStagingDirs(); err != nil {
			_ = os.Remove(path)
			return nil, err
		}
		return func() { _ = os.Remove(path) }, nil
	}
}

func lockPID(path string) (int, error) {
	// #nosec G304 -- the caller supplies the fixed lock path under the plugin directory.
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func (m *Manager) cleanStagingDirs() error {
	entries, err := os.ReadDir(m.pluginsDir())
	if err != nil {
		return SystemError(CodeIOError, err.Error(), "")
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".staging-") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(m.pluginsDir(), entry.Name())); err != nil {
			return SystemError(CodeIOError, err.Error(), "")
		}
	}
	return nil
}
