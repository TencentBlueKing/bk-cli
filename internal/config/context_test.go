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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/config"
)

var _ = Describe("Context", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-ctx-*")
		Expect(err).NotTo(HaveOccurred())
		os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)
		DeferCleanup(func() {
			os.Unsetenv("BK_CLI_CONFIG_DIR")
			os.RemoveAll(tmpDir)
		})
	})

	Describe("ResolveContext without existing contexts", func() {
		It("returns an initialization error when no contexts exist", func() {
			_, _, err := config.ResolveContext("")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("bk-cli context init --bk_api_url_tmpl=URL"))
		})
	})

	Describe("ResolveContext", func() {
		BeforeEach(func() {
			cfg := &config.Config{
				BkAPIURLTmpl: "https://example.com/api/{gateway_name}/",
				UserKey:      "bk_token",
			}
			Expect(config.CreateContext("prod", cfg)).To(Succeed())
			Expect(config.SetActiveContext("prod")).To(Succeed())
		})

		It("resolves active context when no override", func() {
			name, cfg, err := config.ResolveContext("")
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("prod"))
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.BkAPIURLTmpl).To(Equal("https://example.com/api/{gateway_name}/"))
		})

		It("resolves override context", func() {
			cfg2 := &config.Config{
				BkAPIURLTmpl: "https://staging.example.com/api/{gateway_name}/",
				UserKey:      "bk_token",
			}
			Expect(config.CreateContext("staging", cfg2)).To(Succeed())

			name, cfg, err := config.ResolveContext("staging")
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("staging"))
			Expect(cfg.BkAPIURLTmpl).To(Equal("https://staging.example.com/api/{gateway_name}/"))
		})

		It("returns error on non-existent override", func() {
			_, _, err := config.ResolveContext("nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(
				err.Error(),
			).To(
				ContainSubstring("bk-cli context create nonexistent --bk_api_url_tmpl=URL"),
			)
		})

		It("rejects an invalid override context name", func() {
			_, _, err := config.ResolveContext("../escape")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context name"))
		})

		It("suggests context init when the missing override is default", func() {
			_, _, err := config.ResolveContext("default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("bk-cli context init --bk_api_url_tmpl=URL"))
		})

		It("rejects an invalid active context name", func() {
			Expect(config.SetActiveContext("../escape")).To(Succeed())

			_, _, err := config.ResolveContext("")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context name"))
		})
	})

	Describe("ListContexts", func() {
		It("returns empty list when no contexts", func() {
			contexts, err := config.ListContexts()
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).To(BeEmpty())
		})

		It("returns multiple contexts", func() {
			cfg := &config.Config{
				BkAPIURLTmpl: "https://example.com/api/{gateway_name}/",
			}
			Expect(config.CreateContext("alpha", cfg)).To(Succeed())
			Expect(config.CreateContext("beta", cfg)).To(Succeed())

			contexts, err := config.ListContexts()
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).To(HaveLen(2))
			Expect(contexts).To(ContainElements("alpha", "beta"))
		})
	})

	Describe("CreateContext", func() {
		It("creates directory and config.yaml", func() {
			cfg := &config.Config{
				BkAPIURLTmpl: "https://example.com/api/{gateway_name}/",
				UserKey:      "bk_token",
			}
			Expect(config.CreateContext("newctx", cfg)).To(Succeed())
			Expect(config.ContextExists("newctx")).To(BeTrue())

			// Verify config.yaml exists
			configPath := filepath.Join(tmpDir, "contexts", "newctx", "config.yaml")
			_, err := os.Stat(configPath)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error on duplicate", func() {
			cfg := &config.Config{
				BkAPIURLTmpl: "https://example.com/api/{gateway_name}/",
			}
			Expect(config.CreateContext("dup", cfg)).To(Succeed())
			err := config.CreateContext("dup", cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already exists"))
		})

		It("rejects invalid context names", func() {
			cfg := &config.Config{
				BkAPIURLTmpl: "https://example.com/api/{gateway_name}/",
			}
			err := config.CreateContext("../escape", cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context name"))
		})
	})

	Describe("DeleteContext", func() {
		It("removes directory", func() {
			cfg := &config.Config{
				BkAPIURLTmpl: "https://example.com/api/{gateway_name}/",
			}
			Expect(config.CreateContext("todelete", cfg)).To(Succeed())
			Expect(config.ContextExists("todelete")).To(BeTrue())

			Expect(config.DeleteContext("todelete")).To(Succeed())
			Expect(config.ContextExists("todelete")).To(BeFalse())
		})

		It("returns error on non-existent", func() {
			err := config.DeleteContext("ghost")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not exist"))
		})

		It("rejects invalid context names", func() {
			err := config.DeleteContext("../escape")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context name"))
		})
	})

	Describe("SetActiveContext/ActiveContextName", func() {
		It("round-trips active context name", func() {
			Expect(config.SetActiveContext("mycontext")).To(Succeed())

			name, err := config.ActiveContextName()
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("mycontext"))
		})
	})
})
