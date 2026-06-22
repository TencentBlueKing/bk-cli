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

package config_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/config"
)

var _ = Describe("Config", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-test-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, tmpDir)
	})

	Describe("Load", func() {
		It("loads valid YAML", func() {
			content := `bk_api_url_tmpl: "https://example.com/api/{gateway_name}/"
bk_auth_url: "https://auth.example.com"
tenant_id: "test-tenant"
user_key: "bk_ticket"
`
			path := filepath.Join(tmpDir, "config.yaml")
			Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

			cfg, err := config.Load(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.BkAPIURLTmpl).To(Equal("https://example.com/api/{gateway_name}/"))
			Expect(cfg.BkAuthURL).To(Equal("https://auth.example.com"))
			Expect(cfg.TenantID).To(Equal("test-tenant"))
			Expect(cfg.UserKey).To(Equal("bk_ticket"))
		})

		It("returns error for missing file", func() {
			_, err := config.Load(filepath.Join(tmpDir, "nonexistent.yaml"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read config"))
		})

		It("returns error for invalid YAML", func() {
			path := filepath.Join(tmpDir, "bad.yaml")
			Expect(os.WriteFile(path, []byte("bk_api_url_tmpl:\n\t- bad\n\t\tindent"), 0o600)).To(Succeed())

			_, err := config.Load(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse config"))
		})

		It("defaults empty user_key to bk_token", func() {
			content := `bk_api_url_tmpl: "https://example.com/api/{gateway_name}/"
`
			path := filepath.Join(tmpDir, "config.yaml")
			Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

			cfg, err := config.Load(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.UserKey).To(Equal("bk_token"))
		})

		It("defaults empty timeout to 60s", func() {
			content := `bk_api_url_tmpl: "https://example.com/api/{gateway_name}/"
`
			path := filepath.Join(tmpDir, "config.yaml")
			Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

			cfg, err := config.Load(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Timeout).To(Equal(60 * time.Second))
		})

		It("loads an explicit timeout", func() {
			content := `bk_api_url_tmpl: "https://example.com/api/{gateway_name}/"
timeout: 90s
`
			path := filepath.Join(tmpDir, "config.yaml")
			Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

			cfg, err := config.Load(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Timeout).To(Equal(90 * time.Second))
		})

		It("rejects negative timeouts when loading", func() {
			content := `bk_api_url_tmpl: "https://example.com/api/{gateway_name}/"
timeout: -1s
`
			path := filepath.Join(tmpDir, "config.yaml")
			Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

			_, err := config.Load(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timeout must be greater than or equal to 0"))
		})
	})

	Describe("Save", func() {
		It("writes valid YAML that can be loaded back", func() {
			cfg := &config.Config{
				BkAPIURLTmpl: "https://example.com/api/{gateway_name}/",
				UserKey:      "bk_token",
				Timeout:      75 * time.Second,
			}
			path := filepath.Join(tmpDir, "config.yaml")
			Expect(cfg.Save(path)).To(Succeed())

			loaded, err := config.Load(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded.BkAPIURLTmpl).To(Equal(cfg.BkAPIURLTmpl))
			Expect(loaded.UserKey).To(Equal(cfg.UserKey))
			Expect(loaded.Timeout).To(Equal(cfg.Timeout))
		})

		It("creates parent directories", func() {
			cfg := &config.Config{
				BkAPIURLTmpl: "https://example.com/api/{gateway_name}/",
			}
			path := filepath.Join(tmpDir, "a", "b", "c", "config.yaml")
			Expect(cfg.Save(path)).To(Succeed())

			_, err := os.Stat(path)
			Expect(err).NotTo(HaveOccurred())
		})

		It("normalizes legacy {api_name} placeholder before writing", func() {
			cfg := &config.Config{
				BkAPIURLTmpl: "https://example.com/api/{api_name}/",
			}
			path := filepath.Join(tmpDir, "config.yaml")
			Expect(cfg.Save(path)).To(Succeed())

			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("https://example.com/api/{gateway_name}/"))
			Expect(string(data)).NotTo(ContainSubstring("{api_name}"))
		})

		It("rejects negative timeouts before writing", func() {
			cfg := &config.Config{
				BkAPIURLTmpl: "https://example.com/api/{gateway_name}/",
				Timeout:      -1 * time.Second,
			}

			err := cfg.Save(filepath.Join(tmpDir, "config.yaml"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timeout must be greater than or equal to 0"))
		})

		It("returns an error when the config directory cannot be created", func() {
			blocker := filepath.Join(tmpDir, "blocked")
			Expect(os.WriteFile(blocker, []byte("file"), 0o600)).To(Succeed())

			cfg := &config.Config{BkAPIURLTmpl: "https://example.com/api/{gateway_name}/"}
			err := cfg.Save(filepath.Join(blocker, "config.yaml"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create config directory"))
		})
	})

	Describe("ValidateURLTemplate", func() {
		It("accepts valid subdomain pattern", func() {
			err := config.ValidateURLTemplate("https://{gateway_name}.example.com/")
			Expect(err).NotTo(HaveOccurred())
		})

		It("accepts valid path-based pattern", func() {
			err := config.ValidateURLTemplate("https://example.com/api/{gateway_name}/")
			Expect(err).NotTo(HaveOccurred())
		})

		It("accepts the legacy {api_name} placeholder", func() {
			err := config.ValidateURLTemplate("https://example.com/api/{api_name}/")
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects missing {gateway_name}", func() {
			err := config.ValidateURLTemplate("https://example.com/api/")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("{gateway_name}"))
		})

		It("rejects empty string", func() {
			err := config.ValidateURLTemplate("")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("required"))
		})

		It("rejects URL without scheme", func() {
			err := config.ValidateURLTemplate("example.com/{gateway_name}/")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("scheme"))
		})
	})
})
