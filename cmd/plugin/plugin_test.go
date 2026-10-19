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
	"bytes"
	"errors"
	"os"
	"path/filepath"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	pluginlib "github.com/TencentBlueKing/bk-cli/internal/plugin"
)

type envelope struct {
	OK     bool           `json:"ok"`
	DryRun bool           `json:"dry_run"`
	Data   map[string]any `json:"data"`
	Error  *struct {
		Code string `json:"code"`
		Hint string `json:"hint"`
	} `json:"error"`
}

func run(m *pluginlib.Manager, dryRun, insecure bool, args ...string) (envelope, envelope, error) {
	cmd := NewPluginCmd(m, func() bool { return dryRun }, func() bool { return insecure })
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	var o, e envelope
	if out.Len() > 0 {
		Expect(json.Unmarshal(out.Bytes(), &o)).To(Succeed(), out.String())
	}
	if errOut.Len() > 0 {
		Expect(json.Unmarshal(errOut.Bytes(), &e)).To(Succeed(), errOut.String())
	}
	return o, e, err
}

const listAuthCatalogYAML = `schema_version: 1
plugins:
  bkms:
    binary: bkms-cli
    description: test
    recommended_version: v1.0.4
    versions:
      v1.0.4:
        status: allowed
        auth: none
        platforms:
          linux-amd64:
            url: https://example.com/bkms-cli_1.0.4_linux_amd64.tar.gz
            archive_sha256: 2db9bc039a6b209e34d12d229be77f05936e7583ccd04fcddebc79f479b82fc1
            executable: bkms-cli
            executable_sha256: 16830a59a684b2b4ab02d67372f6682b7bb4d4e12d2eefdb84ee93339c9bc5bf
      v1.0.3:
        status: revoked
        auth: shared
`

func writeInstalled(base, name, version string) {
	pluginsDir := filepath.Join(base, "plugins")
	Expect(os.MkdirAll(pluginsDir, 0o755)).To(Succeed())
	data := "plugins:\n  " + name + ": " + version + "\n"
	Expect(os.WriteFile(filepath.Join(pluginsDir, "installed.yaml"), []byte(data), 0o644)).To(Succeed())
}

var _ = Describe("plugin command", func() {
	var m *pluginlib.Manager
	var base string
	BeforeEach(func() {
		c, err := pluginlib.LoadCatalog()
		Expect(err).NotTo(HaveOccurred())
		base = filepath.Join(GinkgoT().TempDir(), "not-created")
		m = pluginlib.NewManager(c, base, "linux", "amd64")
	})

	It("lists catalog plugins without creating the config directory", func() {
		o, _, err := run(m, false, false, "list")
		Expect(err).NotTo(HaveOccurred())
		Expect(o.OK).To(BeTrue())
		plugins := o.Data["plugins"].([]any)
		Expect(plugins).To(HaveLen(1))
		bkms := plugins[0].(map[string]any)
		Expect(bkms["name"]).To(Equal("bkms"))
		Expect(bkms["recommended_version"]).To(Equal("v1.0.4"))
		Expect(bkms["installed_status"]).To(Equal("not_installed"))
		Expect(bkms["auth"]).To(Equal("none"))
		_, err = os.Stat(base)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	It("ignores dry-run for list", func() {
		o, _, err := run(m, true, false, "list")
		Expect(err).NotTo(HaveOccurred())
		Expect(o.DryRun).To(BeFalse())
		Expect(o.OK).To(BeTrue())
		Expect(o.Data["plugins"]).NotTo(BeNil())
	})

	It("uses installed version auth for revoked installs", func() {
		c, err := pluginlib.ParseCatalog([]byte(listAuthCatalogYAML))
		Expect(err).NotTo(HaveOccurred())
		base = GinkgoT().TempDir()
		writeInstalled(base, "bkms", "v1.0.3")
		m = pluginlib.NewManager(c, base, "linux", "amd64")

		o, _, err := run(m, false, false, "list")
		Expect(err).NotTo(HaveOccurred())
		plugins := o.Data["plugins"].([]any)
		bkms := plugins[0].(map[string]any)
		Expect(bkms["installed_version"]).To(Equal("v1.0.3"))
		Expect(bkms["installed_status"]).To(Equal("not_allowed"))
		Expect(bkms["auth"]).To(Equal("shared"))
		Expect(bkms["recommended_version"]).To(Equal("v1.0.4"))
	})

	It("previews install without touching disk or network", func() {
		o, _, err := run(m, true, false, "install", "bkms", "--version", "v1.0.4")
		Expect(err).NotTo(HaveOccurred())
		Expect(o.DryRun).To(BeTrue())
		Expect(o.Data["operation"]).To(Equal("install"))
		Expect(o.Data["version"]).To(Equal("v1.0.4"))
		Expect(o.Data["source"]).To(HavePrefix("https://github.com/"))
		_, err = os.Stat(base)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	It("rejects unapproved versions with an upgrade hint", func() {
		_, e, err := run(m, false, false, "install", "bkms", "--version", "v9.9.9")
		var cliErr *output.CLIError
		Expect(errors.As(err, &cliErr)).To(BeTrue())
		Expect(cliErr.ExitCode).To(Equal(1))
		Expect(e.Error.Code).To(Equal("plugin_version_not_allowed"))
		Expect(e.Error.Hint).To(ContainSubstring("upgrade bk-cli"))
	})

	It("suggests the v-prefixed catalog version", func() {
		_, e, err := run(m, false, false, "install", "bkms", "--version", "1.0.4")
		var cliErr *output.CLIError
		Expect(errors.As(err, &cliErr)).To(BeTrue())
		Expect(cliErr.ExitCode).To(Equal(1))
		Expect(e.Error.Code).To(Equal("plugin_version_not_allowed"))
		Expect(e.Error.Hint).To(ContainSubstring("v1.0.4"))
	})

	It("rejects --insecure for downloads", func() {
		_, e, err := run(m, false, true, "install", "bkms")
		Expect(err).To(HaveOccurred())
		Expect(e.Error.Code).To(Equal("plugin_unsupported_host_flag"))
	})

	It("reports update of a missing plugin", func() {
		_, e, err := run(m, false, false, "update", "bkms")
		Expect(err).To(HaveOccurred())
		Expect(e.Error.Code).To(Equal("plugin_not_installed"))
	})

	It("removes idempotently", func() {
		o, _, err := run(m, false, false, "remove", "bkms")
		Expect(err).NotTo(HaveOccurred())
		Expect(o.Data["removed"]).To(BeFalse())
	})

	DescribeTable("has no bypass flags or loose args",
		func(args ...string) {
			_, _, err := run(m, false, false, args...)
			Expect(err).To(HaveOccurred())
		},
		Entry("missing name", "install"),
		Entry("extra arg", "install", "bkms", "extra"),
		Entry("share-auth", "install", "bkms", "--share-auth"),
		Entry("binary", "install", "bkms", "--binary", "/bin/sh"),
		Entry("url", "install", "bkms", "--url", "https://x"),
		Entry("skip checksum", "install", "bkms", "--skip-checksum"),
	)
})

var _ = Describe("ReportError", func() {
	It("writes an envelope and forces the requested exit code", func() {
		var buf bytes.Buffer
		err := ReportError(
			&buf,
			pluginlib.UserError("plugin_not_installed", "missing", "install it"),
			pluginlib.ExitHostFailure,
		)
		var cliErr *output.CLIError
		Expect(errors.As(err, &cliErr)).To(BeTrue())
		Expect(cliErr.ExitCode).To(Equal(125))
		Expect(buf.String()).To(ContainSubstring(`"plugin_not_installed"`))
	})
	It("passes plugin exit status through untouched", func() {
		var buf bytes.Buffer
		status := &pluginlib.ExitStatus{Code: 3}
		Expect(ReportError(&buf, status, 0)).To(BeIdenticalTo(status))
		Expect(buf.Len()).To(BeZero())
	})
})
