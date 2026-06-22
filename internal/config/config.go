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

// Package config manages CLI context configuration stored on disk.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config represents per-context CLI settings.
type Config struct {
	BkAPIURLTmpl string        `yaml:"bk_api_url_tmpl"`
	BkAuthURL    string        `yaml:"bk_auth_url,omitempty"`
	TenantID     string        `yaml:"tenant_id,omitempty"`
	UserKey      string        `yaml:"user_key,omitempty"`
	Timeout      time.Duration `yaml:"timeout,omitempty"`
}

// NormalizeURLTemplate rewrites legacy placeholders to the canonical form.
func NormalizeURLTemplate(tmpl string) string {
	return strings.ReplaceAll(tmpl, "{api_name}", "{gateway_name}")
}

// Load reads a config from the given YAML file path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	if err := cfg.ApplyDefaults(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes the config to the given YAML file path.
func (c *Config) Save(path string) error {
	copy := *c
	if err := copy.ApplyDefaults(); err != nil {
		return err
	}

	data, err := yaml.Marshal(&copy)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// ApplyDefaults normalizes legacy values and fills unset defaults.
func (c *Config) ApplyDefaults() error {
	c.BkAPIURLTmpl = NormalizeURLTemplate(c.BkAPIURLTmpl)
	if c.UserKey == "" {
		c.UserKey = DefaultUserKey
	}
	if c.Timeout < 0 {
		return fmt.Errorf("timeout must be greater than or equal to 0")
	}
	if c.Timeout == 0 {
		c.Timeout = DefaultTimeout
	}
	return nil
}

// ValidateURLTemplate checks that a BK_API_URL_TMPL is valid.
func ValidateURLTemplate(tmpl string) error {
	tmpl = NormalizeURLTemplate(tmpl)

	if tmpl == "" {
		return fmt.Errorf("bk_api_url_tmpl is required")
	}
	if !strings.Contains(tmpl, "{gateway_name}") {
		return fmt.Errorf("bk_api_url_tmpl must contain {gateway_name} placeholder")
	}
	// Replace placeholder to test URL validity
	testURL := strings.ReplaceAll(tmpl, "{gateway_name}", "test")
	parsed, err := url.Parse(testURL)
	if err != nil {
		return fmt.Errorf("bk_api_url_tmpl is not a valid URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf(
			"bk_api_url_tmpl must include scheme and host (e.g., https://example.com/api/{gateway_name}/)",
		)
	}
	return nil
}
