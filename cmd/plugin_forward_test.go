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

package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	pluginlib "github.com/TencentBlueKing/bk-cli/internal/plugin"
)

var names = map[string]bool{"bkms": true}

var _ = Describe("splitPluginInvocation", func() {
	DescribeTable("matches plugin calls and keeps child args verbatim",
		func(args []string, want pluginInvocation) {
			got, ok := splitPluginInvocation(args, names)
			Expect(ok).To(BeTrue())
			Expect(got).To(Equal(want))
		},
		Entry("plain", []string{"bkms", "app", "list"},
			pluginInvocation{Name: "bkms", Args: []string{"app", "list"}}),
		Entry("no args", []string{"bkms"}, pluginInvocation{Name: "bkms", Args: []string{}}),
		Entry("context prefix", []string{"--context", "clouds", "bkms", "app", "--context", "child"},
			pluginInvocation{Name: "bkms", Context: "clouds", Args: []string{"app", "--context", "child"}}),
		Entry("context equals", []string{"--context=clouds", "bkms", "--help"},
			pluginInvocation{Name: "bkms", Context: "clouds", Args: []string{"--help"}}),
		Entry("context named like plugin", []string{"--context", "bkms", "bkms", "list"},
			pluginInvocation{Name: "bkms", Context: "bkms", Args: []string{"list"}}),
		Entry("child terminator and empties", []string{"bkms", "--", "", "two words", "--dry-run"},
			pluginInvocation{Name: "bkms", Args: []string{"--", "", "two words", "--dry-run"}}),
		Entry("host dry-run", []string{"--dry-run", "bkms", "app"},
			pluginInvocation{Name: "bkms", Args: []string{"app"}, BadFlag: "--dry-run"}),
		Entry("host verbose short", []string{"-v", "bkms"},
			pluginInvocation{Name: "bkms", Args: []string{}, BadFlag: "-v"}),
		Entry("host insecure", []string{"--insecure", "--context", "x", "bkms"},
			pluginInvocation{Name: "bkms", Context: "x", Args: []string{}, BadFlag: "--insecure"}),
	)

	DescribeTable("leaves non-plugin calls to Cobra",
		func(args ...string) {
			_, ok := splitPluginInvocation(args, names)
			Expect(ok).To(BeFalse())
		},
		Entry("builtin", "api", "bk-apigateway", "GET", "/"),
		Entry("dry-run builtin", "--dry-run", "api", "bkms"),
		Entry("unknown", "nope"),
		Entry("unknown flag", "--bogus", "bkms"),
		Entry("terminator", "--", "bkms"),
		Entry("long help before plugin", "--help", "bkms"),
		Entry("short help before plugin", "-h", "bkms"),
		Entry("empty"),
		Entry("context without value", "--context"),
	)
})

var _ = Describe("root plugin wiring", func() {
	BeforeEach(func() {
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", GinkgoT().TempDir())
	})

	It("attaches every catalog plugin without name conflicts", func() {
		catalog, err := pluginlib.LoadCatalog()
		Expect(err).NotTo(HaveOccurred())
		root := newRootCmdWithPluginManager(
			pluginlib.NewManager(catalog, os.Getenv("BK_CLI_CONFIG_DIR"), runtime.GOOS, runtime.GOARCH),
		)
		for _, name := range catalog.Names() {
			found, _, findErr := root.Find([]string{name})
			Expect(findErr).NotTo(HaveOccurred())
			Expect(found.Annotations).To(HaveKeyWithValue(pluginAnnotation, "true"))
		}
	})

	It("skips catalog names that collide with built-in commands", func() {
		catalog := &pluginlib.Catalog{SchemaVersion: 1, Plugins: map[string]pluginlib.Definition{
			"api":  {Binary: "x", RecommendedVersion: "v1.0.0"},
			"help": {Binary: "x", RecommendedVersion: "v1.0.0"},
		}}
		root := newRootCmd()
		skipped := attachPluginCommands(
			root,
			pluginlib.NewManager(catalog, GinkgoT().TempDir(), "linux", "amd64"),
		)
		Expect(skipped).To(ConsistOf("api", "help"))
	})

	It("routes a catalog name collision to the built-in command", func() {
		catalog := &pluginlib.Catalog{SchemaVersion: 1, Plugins: map[string]pluginlib.Definition{
			"api": {Binary: "api-plugin", Description: "Must not replace the built-in API command"},
		}}
		manager := pluginlib.NewManager(catalog, GinkgoT().TempDir(), "linux", "amd64")
		root := newRootCmdWithPluginManager(manager)
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)

		Expect(executeRoot(root, []string{"api", "--help"})).To(Succeed())
		Expect(stdout.String()).To(ContainSubstring("Make direct HTTP calls to any BlueKing API gateway"))
		Expect(stderr.String()).NotTo(ContainSubstring("plugin_not_installed"))
	})

	It("shows plugins in root help with install status", func() {
		root := newRootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		Expect(executeRoot(root, []string{"--help"})).To(Succeed())
		Expect(out.String()).To(ContainSubstring("bkms"))
		Expect(out.String()).To(ContainSubstring("[plugin]"))
		Expect(out.String()).To(ContainSubstring("not installed"))
	})

	It("lets Cobra handle host help before a plugin name", func() {
		root := newRootCmd()
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)

		Expect(executeRoot(root, []string{"--help", "bkms"})).To(Succeed())
		Expect(stdout.String()).To(ContainSubstring("Available Commands:"))
		Expect(stdout.String()).To(ContainSubstring("Usage:\n  bk-cli [flags]"))
		Expect(stderr.String()).NotTo(ContainSubstring("plugin_not_installed"))
	})

	It("shows unreadable install state in plugin help", func() {
		base := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(base, "plugins"), 0o700)).To(Succeed())
		Expect(os.WriteFile(
			filepath.Join(base, "plugins", "installed.yaml"),
			[]byte("plugins: ["),
			0o600,
		)).To(Succeed())
		catalog := &pluginlib.Catalog{SchemaVersion: 1, Plugins: map[string]pluginlib.Definition{
			"bkms": {Binary: "bkms-cli", Description: "BlueKing service governance CLI"},
		}}
		root := newRootCmdWithPluginManager(pluginlib.NewManager(catalog, base, "linux", "amd64"))
		var out bytes.Buffer
		root.SetOut(&out)

		Expect(root.Help()).To(Succeed())
		Expect(out.String()).To(ContainSubstring("install state unreadable"))
		Expect(out.String()).NotTo(ContainSubstring("(not installed)"))
	})

	It("shows plugin stub details through the help command", func() {
		catalog, err := pluginlib.LoadCatalog()
		Expect(err).NotTo(HaveOccurred())
		root := newRootCmd()
		var out bytes.Buffer
		root.SetOut(&out)

		Expect(executeRoot(root, []string{"help", "bkms"})).To(Succeed())
		Expect(out.String()).To(ContainSubstring(catalog.Plugins["bkms"].Description))
		Expect(out.String()).To(ContainSubstring("not installed"))
		Expect(out.String()).To(ContainSubstring("bk-cli bkms --help"))
	})

	It("fails with 125 when the plugin is not installed", func() {
		root := newRootCmd()
		var stderr bytes.Buffer
		root.SetErr(&stderr)
		err := executeRoot(root, []string{"bkms", "--help"})
		Expect(cliExitCode(err)).To(Equal(125))
		Expect(stderr.String()).To(ContainSubstring(`"plugin_not_installed"`))
	})

	It("rejects host flags before the plugin with 125", func() {
		root := newRootCmd()
		var stderr bytes.Buffer
		root.SetErr(&stderr)
		err := executeRoot(root, []string{"--dry-run", "bkms", "app", "list"})
		Expect(cliExitCode(err)).To(Equal(125))
		Expect(stderr.String()).To(ContainSubstring(`"plugin_unsupported_host_flag"`))
	})

	It("rejects combined host flags that reach the plugin stub with 125", func() {
		root := newRootCmd()
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)

		err := executeRoot(root, []string{"-vv", "bkms", "app", "delete", "x"})
		Expect(cliExitCode(err)).To(Equal(125))
		Expect(stderr.String()).To(ContainSubstring(`"plugin_unsupported_host_flag"`))
		Expect(stdout.String()).To(BeEmpty())
	})

	It("keeps existing system routing untouched", func() {
		root := newRootCmd()
		err := executeRoot(root, []string{"sops", "start_taks", "-h"})
		Expect(err).To(MatchError(ContainSubstring(`unknown command "start_taks" for "bk-cli sops"`)))
	})

	It("forwards to the installed plugin through executeRoot", func() {
		manager := installProbeForRoot()
		root := newRootCmdWithPluginManager(manager)
		var stdout bytes.Buffer
		root.SetOut(&stdout)
		root.SetIn(bytes.NewReader(nil))
		Expect(executeRoot(root, []string{"bkms", "a", "", "--dry-run"})).To(Succeed())
		var got struct {
			Args []string `json:"args"`
		}
		Expect(json.Unmarshal(stdout.Bytes(), &got)).To(Succeed())
		Expect(got.Args).To(Equal([]string{"a", "", "--dry-run"}))
	})
})

func cliExitCode(err error) int {
	var cliErr *output.CLIError
	if errors.As(err, &cliErr) {
		return cliErr.ExitCode
	}
	return -1
}

func installProbeForRoot() *pluginlib.Manager {
	if runtime.GOOS == "windows" {
		Skip("probe forwarding is covered by internal/plugin runner tests on Windows")
	}
	dir := GinkgoT().TempDir()
	binary := filepath.Join(dir, "bkms-cli")
	build := exec.Command("go", "build", "-o", binary, "../internal/plugin/testdata/probe")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := build.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), string(out))
	body, err := os.ReadFile(binary)
	Expect(err).NotTo(HaveOccurred())

	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	Expect(tw.WriteHeader(&tar.Header{
		Name: "bkms-cli", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg,
	})).To(Succeed())
	_, err = tw.Write(body)
	Expect(err).NotTo(HaveOccurred())
	Expect(tw.Close()).To(Succeed())
	Expect(gz.Close()).To(Succeed())
	archivePath := filepath.Join(dir, "probe.tar.gz")
	Expect(os.WriteFile(archivePath, archive.Bytes(), 0o600)).To(Succeed())

	archiveSum, err := pluginlib.FileSHA256(archivePath)
	Expect(err).NotTo(HaveOccurred())
	exeSum, err := pluginlib.FileSHA256(binary)
	Expect(err).NotTo(HaveOccurred())
	platform := runtime.GOOS + "-" + runtime.GOARCH
	catalog := &pluginlib.Catalog{SchemaVersion: 1, Plugins: map[string]pluginlib.Definition{
		"bkms": {
			Binary: "bkms-cli", RecommendedVersion: "v1.0.4", Versions: map[string]pluginlib.Release{
				"v1.0.4": {
					Status: "allowed", Auth: "none", Platforms: map[string]pluginlib.Asset{
						platform: {
							URL:              "https://example.invalid/probe.tar.gz",
							ArchiveSHA256:    archiveSum,
							Executable:       "bkms-cli",
							ExecutableSHA256: exeSum,
						},
					},
				},
			},
		},
	}}
	manager := pluginlib.NewManager(catalog, os.Getenv("BK_CLI_CONFIG_DIR"), runtime.GOOS, runtime.GOARCH)
	_, err = manager.Install(context.Background(), "bkms", pluginlib.InstallOptions{FromFile: archivePath})
	Expect(err).NotTo(HaveOccurred())
	return manager
}
