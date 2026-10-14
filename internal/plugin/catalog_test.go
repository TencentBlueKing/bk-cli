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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const validCatalogYAML = `schema_version: 1
plugins:
  bkms:
    binary: bkms-cli
    description: test
    recommended_version: v1.0.4
    versions:
      v1.0.4:
        status: allowed
        platforms:
          linux-amd64:
            url: https://example.com/bkms-cli_1.0.4_linux_amd64.tar.gz
            archive_sha256: 2db9bc039a6b209e34d12d229be77f05936e7583ccd04fcddebc79f479b82fc1
            executable: bkms-cli
            executable_sha256: 16830a59a684b2b4ab02d67372f6682b7bb4d4e12d2eefdb84ee93339c9bc5bf
      v1.0.3:
        status: revoked
`

func expectPluginError(err error, code string) *Error {
	var pErr *Error
	ExpectWithOffset(1, errors.As(err, &pErr)).To(BeTrue(), "error %v is not *plugin.Error", err)
	ExpectWithOffset(1, pErr.Code).To(Equal(code))
	return pErr
}

var _ = Describe("embedded catalog", func() {
	It("resolves the recommended bkms release for linux-amd64 with auth none", func() {
		c, err := LoadCatalog()
		Expect(err).NotTo(HaveOccurred())
		r, err := c.Resolve("bkms", "", "linux-amd64")
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Version).To(Equal("v1.0.4"))
		Expect(r.Release.Auth).To(Equal("none"))
		Expect(
			r.Asset.ExecutableSHA256,
		).To(
			Equal("16830a59a684b2b4ab02d67372f6682b7bb4d4e12d2eefdb84ee93339c9bc5bf"),
		)
		Expect(c.Names()).To(Equal([]string{"bkms"}))
	})

	It("lists every supported platform for v1.0.4", func() {
		c, err := LoadCatalog()
		Expect(err).NotTo(HaveOccurred())
		for _, p := range SupportedPlatforms {
			_, err := c.Resolve("bkms", "v1.0.4", p)
			Expect(err).NotTo(HaveOccurred(), p)
		}
	})
})

var _ = Describe("catalog resolve", func() {
	var c *Catalog
	BeforeEach(func() {
		var err error
		c, err = ParseCatalog([]byte(validCatalogYAML))
		Expect(err).NotTo(HaveOccurred())
	})

	It("defaults auth to none", func() {
		r, err := c.Resolve("bkms", "v1.0.4", "linux-amd64")
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Release.Auth).To(Equal("none"))
	})
	It("rejects unknown plugins", func() {
		_, err := c.Resolve("nope", "", "linux-amd64")
		expectPluginError(err, CodeUnknown)
	})
	It("suggests the catalog version when only the leading v is missing", func() {
		_, err := c.Resolve("bkms", "1.0.4", "linux-amd64")
		pErr := expectPluginError(err, CodeVersionNotAllowed)
		Expect(pErr.Hint).To(ContainSubstring("v1.0.4"))
	})
	DescribeTable("rejects versions that are not allowed",
		func(version string) {
			_, err := c.Resolve("bkms", version, "linux-amd64")
			expectPluginError(err, CodeVersionNotAllowed)
		},
		Entry("unlisted", "v9.9.9"),
		Entry("revoked", "v1.0.3"),
		Entry("latest", "latest"),
		Entry("path", "../v1.0.4"),
	)
	It("rejects unlisted platforms", func() {
		_, err := c.Resolve("bkms", "v1.0.4", "darwin-arm64")
		expectPluginError(err, CodePlatformUnsupported)
	})
})

var _ = Describe("catalog parse", func() {
	DescribeTable(
		"rejects invalid catalogs",
		func(old, replacement string) {
			_, err := ParseCatalog([]byte(strings.Replace(validCatalogYAML, old, replacement, 1)))
			expectPluginError(err, CodeCatalogInvalid)
		},
		Entry("schema version", "schema_version: 1", "schema_version: 2"),
		Entry("unknown field", "binary: bkms-cli", "binary: bkms-cli\n    install: npm i -g x"),
		Entry("unsafe name", "  bkms:", "  ../bkms:"),
		Entry("http url", "https://example.com", "http://example.com"),
		Entry("uppercase hash", "archive_sha256: 2db9", "archive_sha256: 2DB9"),
		Entry(
			"short hash",
			"executable_sha256: 16830a59a684b2b4ab02d67372f6682b7bb4d4e12d2eefdb84ee93339c9bc5bf",
			"executable_sha256: abc",
		),
		Entry("executable path", "executable: bkms-cli", "executable: bin/bkms-cli"),
		Entry("unknown auth", "status: allowed", "status: allowed\n        auth: all"),
		Entry("unknown auth on revoked", "status: revoked", "status: revoked\n        auth: all"),
		Entry("unknown status", "status: revoked", "status: disabled"),
		Entry("bad platform", "linux-amd64:", "linux-386:"),
		Entry("non-canonical version", "      v1.0.4:", "      v1.0.4.0:"),
		Entry("recommended revoked", "recommended_version: v1.0.4", "recommended_version: v1.0.3"),
		Entry("recommended missing", "recommended_version: v1.0.4", "recommended_version: v2.0.0"),
		Entry("duplicate key", "  bkms:\n", "  bkms:\n    binary: x\n"),
	)

	It("rejects a second YAML document", func() {
		_, err := ParseCatalog([]byte(validCatalogYAML + "---\nschema_version: 1\n"))
		expectPluginError(err, CodeCatalogInvalid)
	})

	It("rejects windows executables without .exe", func() {
		yaml := strings.Replace(validCatalogYAML, "linux-amd64:", "windows-amd64:", 1)
		yaml = strings.Replace(yaml, "linux_amd64.tar.gz", "windows_amd64.zip", 1)
		_, err := ParseCatalog([]byte(yaml))
		expectPluginError(err, CodeCatalogInvalid)
	})
})
