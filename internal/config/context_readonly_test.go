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

var _ = Describe("context read-only helpers", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-context-readonly-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("prefers BK_CLI_CONFIG_DIR for the base directory and credentials path", func() {
		Expect(config.BaseDirectory()).To(Equal(tmpDir))
		Expect(
			config.CredentialsPath("demo"),
		).To(
			Equal(filepath.Join(tmpDir, "contexts", "demo", "credentials.enc")),
		)
	})

	It("falls back to the user home directory when no override is set", func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		home, err := os.UserHomeDir()
		Expect(err).NotTo(HaveOccurred())

		Expect(config.BaseDirectory()).To(Equal(filepath.Join(home, ".bk-cli")))

		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	It("returns nil when no context exists in read-only mode", func() {
		name, cfg, err := config.ResolveContextReadOnly("")
		Expect(err).NotTo(HaveOccurred())
		Expect(name).To(BeEmpty())
		Expect(cfg).To(BeNil())
	})

	It("returns an empty active context name when the marker file is missing", func() {
		name, err := config.ActiveContextName()
		Expect(err).NotTo(HaveOccurred())
		Expect(name).To(BeEmpty())
	})

	It("returns an error when the active context marker cannot be read", func() {
		Expect(os.MkdirAll(filepath.Join(tmpDir, "current"), 0o700)).To(Succeed())

		_, err := config.ActiveContextName()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to read active context"))
	})

	It("loads the active context in read-only mode", func() {
		cfg := &config.Config{BkAPIURLTmpl: "https://example.com/api/{gateway_name}/"}
		Expect(config.CreateContext("default", cfg)).To(Succeed())
		Expect(config.SetActiveContext("default")).To(Succeed())

		name, resolved, err := config.ResolveContextReadOnly("")
		Expect(err).NotTo(HaveOccurred())
		Expect(name).To(Equal("default"))
		Expect(resolved.BkAPIURLTmpl).To(Equal("https://example.com/api/{gateway_name}/"))
	})

	It("returns an error for an explicit missing context in read-only mode", func() {
		_, _, err := config.ResolveContextReadOnly("missing")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`context "missing" not found`))
	})

	It("rejects invalid explicit context names in read-only mode", func() {
		_, _, err := config.ResolveContextReadOnly("../escape")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("context name"))
	})

	It("uses the first existing context when there is no active context", func() {
		cfg := &config.Config{BkAPIURLTmpl: "https://example.com/api/{gateway_name}/"}
		Expect(config.CreateContext("alpha", cfg)).To(Succeed())

		name, resolved, err := config.ResolveContextReadOnly("")
		Expect(err).NotTo(HaveOccurred())
		Expect(name).To(Equal("alpha"))
		Expect(resolved).NotTo(BeNil())
	})

	It("auto-selects the first context and persists it in normal resolve mode", func() {
		cfg := &config.Config{BkAPIURLTmpl: "https://example.com/api/{gateway_name}/"}
		Expect(config.CreateContext("alpha", cfg)).To(Succeed())

		name, _, err := config.ResolveContext("")
		Expect(err).NotTo(HaveOccurred())
		Expect(name).To(Equal("alpha"))

		activeName, err := config.ActiveContextName()
		Expect(err).NotTo(HaveOccurred())
		Expect(activeName).To(Equal("alpha"))
	})

	It("rejects an invalid active context name in read-only mode", func() {
		Expect(config.SetActiveContext("../escape")).To(Succeed())

		_, _, err := config.ResolveContextReadOnly("")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("context name"))
	})

	It("returns a config load error in read-only mode when the context file is missing", func() {
		Expect(os.MkdirAll(filepath.Join(tmpDir, "contexts", "broken"), 0o700)).To(Succeed())
		Expect(config.SetActiveContext("broken")).To(Succeed())

		_, _, err := config.ResolveContextReadOnly("")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to read config"))
	})
})
