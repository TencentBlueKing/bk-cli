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

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

// BaseDirectory returns the base config directory, respecting BK_CLI_CONFIG_DIR.
func BaseDirectory() string {
	if dir := os.Getenv("BK_CLI_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, BaseDir)
}

// ContextDir returns the directory for a named context.
func ContextDir(name string) string {
	return filepath.Join(BaseDirectory(), ContextsDir, name)
}

// ConfigPath returns the config.yaml path for a named context.
func ConfigPath(name string) string {
	return filepath.Join(ContextDir(name), ConfigFileName)
}

// CredentialsPath returns the credentials.enc path for a named context.
func CredentialsPath(name string) string {
	return filepath.Join(ContextDir(name), CredentialsFileName)
}

// ActiveContextName reads the active context from ~/.bk-cli/current.
func ActiveContextName() (string, error) {
	path := filepath.Join(BaseDirectory(), CurrentFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // no active context yet
		}
		return "", fmt.Errorf("failed to read active context: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// SetActiveContext writes the active context name to ~/.bk-cli/current.
func SetActiveContext(name string) error {
	path := filepath.Join(BaseDirectory(), CurrentFileName)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create base directory: %w", err)
	}
	return os.WriteFile(path, []byte(name+"\n"), 0o600)
}

// ListContexts returns names of all contexts in ~/.bk-cli/contexts/.
func ListContexts() ([]string, error) {
	dir := filepath.Join(BaseDirectory(), ContextsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list contexts: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// ContextExists checks if a context directory exists.
func ContextExists(name string) bool {
	info, err := os.Stat(ContextDir(name))
	return err == nil && info.IsDir()
}

// CreateContext creates a new context directory with config.yaml.
func CreateContext(name string, cfg *Config) error {
	if err := validate.ValidateContextName(name); err != nil {
		return err
	}
	if ContextExists(name) {
		return fmt.Errorf("context %q already exists", name)
	}
	return cfg.Save(ConfigPath(name))
}

// DeleteContext removes a context directory and all its files.
func DeleteContext(name string) error {
	if err := validate.ValidateContextName(name); err != nil {
		return err
	}
	if !ContextExists(name) {
		return fmt.Errorf("context %q does not exist", name)
	}
	return os.RemoveAll(ContextDir(name))
}

// ResolveContext resolves the context to use: explicit override or active.
// The first context must be created explicitly with `bk-cli context init`.
func ResolveContext(override string) (string, *Config, error) {
	name := override
	if name == "" {
		active, err := ActiveContextName()
		if err != nil {
			return "", nil, err
		}
		if active != "" {
			if err := validate.ValidateContextName(active); err != nil {
				return "", nil, err
			}
			name = active
		} else {
			contexts, err := ListContexts()
			if err != nil {
				return "", nil, err
			}
			if len(contexts) == 0 {
				return "", nil, fmt.Errorf(
					"no context configured. Initialize one with: bk-cli context init --bk_api_url_tmpl=URL",
				)
			}
			if err := validate.ValidateContextName(contexts[0]); err != nil {
				return "", nil, err
			}
			if err := SetActiveContext(contexts[0]); err != nil {
				return "", nil, err
			}
			name = contexts[0]
		}
	}
	if err := validate.ValidateContextName(name); err != nil {
		return "", nil, err
	}
	if !ContextExists(name) {
		contexts, _ := ListContexts()
		hintCmd := fmt.Sprintf("bk-cli context create %s --bk_api_url_tmpl=URL", name)
		if name == DefaultContextName {
			hintCmd = "bk-cli context init --bk_api_url_tmpl=URL"
		}
		return "", nil, fmt.Errorf(
			"context %q not found. Available: %v. Create with: %s",
			name,
			contexts,
			hintCmd,
		)
	}
	cfg, err := Load(ConfigPath(name))
	if err != nil {
		return "", nil, err
	}
	return name, cfg, nil
}

// ResolveContextReadOnly resolves the context without creating any directories.
// Returns ("", nil, nil) if no context exists and no override was specified.
// Returns an error if an explicit override names a non-existent context.
func ResolveContextReadOnly(override string) (string, *Config, error) {
	name := override
	if name == "" {
		var err error
		name, err = ActiveContextName()
		if err != nil {
			return "", nil, err
		}
		if name == "" {
			// No active context — check if any exist
			contexts, err := ListContexts()
			if err != nil {
				return "", nil, err
			}
			if len(contexts) == 0 {
				return "", nil, nil
			}
			if err := validate.ValidateContextName(contexts[0]); err != nil {
				return "", nil, err
			}
			name = contexts[0]
		}
	}
	if name != "" {
		if err := validate.ValidateContextName(name); err != nil {
			return "", nil, err
		}
	}
	if !ContextExists(name) {
		// Explicit override must exist — return error with available contexts
		if override != "" {
			contexts, _ := ListContexts()
			hintCmd := fmt.Sprintf("bk-cli context create %s --bk_api_url_tmpl=URL", name)
			if name == DefaultContextName {
				hintCmd = "bk-cli context init --bk_api_url_tmpl=URL"
			}
			return "", nil, fmt.Errorf(
				"context %q not found. Available: %v. Create with: %s",
				name,
				contexts,
				hintCmd,
			)
		}
		return "", nil, nil
	}
	cfg, err := Load(ConfigPath(name))
	if err != nil {
		return "", nil, err
	}
	return name, cfg, nil
}
