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
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
)

type probeOutput struct {
	Args     []string `json:"args"`
	Stdin    string   `json:"stdin"`
	Cwd      string   `json:"cwd"`
	Protocol string   `json:"protocol"`
	Context  string   `json:"context"`
	Auth     string   `json:"auth"`
}

var _ = Describe("plugin process", Ordered, func() {
	var probePath string
	var probeBytes []byte

	BeforeAll(func() {
		dir, err := os.MkdirTemp("", "bk-cli-plugin-probe-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, dir)
		probePath = filepath.Join(dir, "bkms-cli")
		if runtime.GOOS == "windows" {
			probePath += ".exe"
		}
		out, err := exec.Command("go", "build", "-o", probePath, "./testdata/probe").CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))
		probeBytes, err = os.ReadFile(probePath)
		Expect(err).NotTo(HaveOccurred())
	})

	// installProbe installs the probe through the real installer from a local archive.
	installProbe := func(auth string) *Manager {
		platform := runtime.GOOS + "-" + runtime.GOARCH
		exe := filepath.Base(probePath)
		archive := archiveFixture(
			ArchiveFormat(platform),
			[]archiveEntry{{Name: exe, Body: string(probeBytes), Mode: 0o755}},
		)
		m := testManager(
			GinkgoT().TempDir(),
			platform,
			"https://unused.invalid",
			releaseFixture{Version: "v1.0.4", Archive: archive, Binary: probeBytes},
		)
		rel := m.Catalog.Plugins["bkms"].Versions["v1.0.4"]
		rel.Auth = auth
		m.Catalog.Plugins["bkms"].Versions["v1.0.4"] = rel
		file := filepath.Join(GinkgoT().TempDir(), "probe."+ArchiveFormat(platform))
		Expect(os.WriteFile(file, archive, 0o600)).To(Succeed())
		_, err := m.Install(context.Background(), "bkms", InstallOptions{FromFile: file})
		Expect(err).NotTo(HaveOccurred())
		return m
	}

	It("passes argv, stdin, stdout and cwd through unchanged", func() {
		var stdout, stderr bytes.Buffer
		args := []string{
			"app",
			"",
			"name with spaces",
			"--flag",
			"false",
			"--",
			"--help",
			"$(literal)",
			"--context",
			"child",
		}
		err := runProcess(
			probePath,
			args,
			[]string{"BK_CLI_PLUGIN_PROTOCOL=1"},
			Streams{In: strings.NewReader("input-body"), Out: &stdout, Err: &stderr},
			nil,
		)
		Expect(err).NotTo(HaveOccurred())
		var got probeOutput
		Expect(json.Unmarshal(stdout.Bytes(), &got)).To(Succeed())
		Expect(got.Args).To(Equal(args))
		Expect(got.Stdin).To(Equal("input-body"))
		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		resolvedCwd, err := filepath.EvalSymlinks(cwd)
		Expect(err).NotTo(HaveOccurred())
		resolvedGotCwd, err := filepath.EvalSymlinks(got.Cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(resolvedGotCwd).To(Equal(resolvedCwd))
		Expect(stderr.String()).To(BeEmpty())
	})

	It("returns the child exit code without adding output", func() {
		var stdout, stderr bytes.Buffer
		err := runProcess(
			probePath,
			[]string{"exit", "7"},
			os.Environ(),
			Streams{In: strings.NewReader(""), Out: &stdout, Err: &stderr},
			nil,
		)
		var status *ExitStatus
		Expect(errors.As(err, &status)).To(BeTrue())
		Expect(status.Code).To(Equal(7))
		Expect(status.Error()).To(Equal("plugin exited with status 7"))
		Expect(stdout.String()).To(BeEmpty())
		Expect(stderr.String()).To(Equal("probe-exit\n"))
	})

	It("reports launch failures as host errors", func() {
		err := runProcess(
			filepath.Join(GinkgoT().TempDir(), "missing"),
			nil,
			nil,
			Streams{In: strings.NewReader("")},
			nil,
		)
		expectPluginError(err, CodeLaunchFailed)
	})

	It("returns 128+signal when the child is killed by a signal", func() {
		if runtime.GOOS == "windows" {
			Skip("signals are unix-only")
		}
		err := runProcess(
			probePath,
			[]string{"kill-self"},
			os.Environ(),
			Streams{In: strings.NewReader(""), Out: io.Discard, Err: io.Discard},
			nil,
		)
		var status *ExitStatus
		Expect(errors.As(err, &status)).To(BeTrue())
		Expect(status.Code).To(Equal(128 + int(syscall.SIGTERM)))
	})

	It("drops SIGINT and SIGQUIT and forwards SIGTERM to the child", func() {
		if runtime.GOOS == "windows" {
			Skip("signals are unix-only")
		}
		pr, pw := io.Pipe()
		signals := make(chan os.Signal, 3)
		done := make(chan error, 1)
		go func() {
			done <- runProcess(
				probePath,
				[]string{"sleep"},
				os.Environ(),
				Streams{In: strings.NewReader(""), Out: pw, Err: io.Discard},
				signals,
			)
			_ = pw.Close()
		}()
		line, err := bufio.NewReader(pr).ReadString('\n')
		Expect(err).NotTo(HaveOccurred())
		Expect(line).To(Equal("ready\n"))
		go func() { _, _ = io.Copy(io.Discard, pr) }()

		signals <- syscall.SIGINT
		Consistently(done, 300*time.Millisecond).ShouldNot(Receive())
		signals <- syscall.SIGQUIT
		Consistently(done, 300*time.Millisecond).ShouldNot(Receive())
		signals <- syscall.SIGTERM
		var runErr error
		Eventually(done, 5*time.Second).Should(Receive(&runErr))
		var status *ExitStatus
		Expect(errors.As(runErr, &status)).To(BeTrue())
		Expect(status.Code).To(Equal(128 + int(syscall.SIGTERM)))
	})

	It("runs a verified install with context and null auth for auth none", func() {
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", GinkgoT().TempDir())
		Expect(config.CreateContext(
			"alpha",
			&config.Config{BkAPIURLTmpl: "https://alpha.example/{gateway_name}/"},
		)).To(Succeed())
		Expect(config.SetActiveContext("alpha")).To(Succeed())
		m := installProbe("none")
		var stdout bytes.Buffer
		Expect(m.Run(
			"bkms",
			"",
			[]string{"list"},
			Streams{In: strings.NewReader(""), Out: &stdout, Err: io.Discard},
		)).To(Succeed())
		var got probeOutput
		Expect(json.Unmarshal(stdout.Bytes(), &got)).To(Succeed())
		Expect(got.Protocol).To(Equal("1"))
		Expect(got.Context).To(MatchJSON(
			`{"name":"alpha","bk_api_url_tmpl":"https://alpha.example/{gateway_name}/"}`,
		))
		Expect(got.Auth).To(Equal("null"))
	})

	It("reports context resolution failures after verifying the executable", func() {
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", GinkgoT().TempDir())
		m := installProbe("none")
		err := m.Run(
			"bkms",
			"missing",
			nil,
			Streams{In: strings.NewReader(""), Out: io.Discard, Err: io.Discard},
		)
		expectPluginError(err, CodeContextError)
	})

	It("shares the selected context credentials for auth shared", func() {
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", GinkgoT().TempDir())
		Expect(config.CreateContext(
			"beta",
			&config.Config{
				BkAPIURLTmpl: "https://beta.example/{gateway_name}/",
				TenantID:     "t1",
			},
		)).To(Succeed())
		key, err := credential.DeriveKey()
		Expect(err).NotTo(HaveOccurred())
		Expect(credential.Save(
			config.CredentialsPath("beta"),
			&credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "app-fake",
				BkAppSecret: "secret-fake",
				BkTicket:    "ticket-fake",
			},
			key,
		)).To(Succeed())
		m := installProbe("shared")
		var stdout bytes.Buffer
		Expect(m.Run(
			"bkms",
			"beta",
			nil,
			Streams{In: strings.NewReader(""), Out: &stdout, Err: io.Discard},
		)).To(Succeed())
		var got probeOutput
		Expect(json.Unmarshal(stdout.Bytes(), &got)).To(Succeed())
		Expect(got.Auth).To(MatchJSON(
			`{"type":"app_user","bk_app_code":"app-fake","bk_app_secret":"secret-fake","bk_ticket":"ticket-fake"}`,
		))
		Expect(got.Context).To(MatchJSON(
			`{"name":"beta","bk_api_url_tmpl":"https://beta.example/{gateway_name}/","tenant_id":"t1"}`,
		))
	})

	It("fails the digest check before reading credentials", func() {
		GinkgoT().Setenv("BK_CLI_CONFIG_DIR", GinkgoT().TempDir())
		Expect(config.CreateContext(
			"alpha",
			&config.Config{BkAPIURLTmpl: "https://alpha.example/{gateway_name}/"},
		)).To(Succeed())
		Expect(os.WriteFile(config.CredentialsPath("alpha"), []byte("corrupted"), 0o600)).To(Succeed())
		m := installProbe("shared")
		_, path, err := m.VerifiedExecutable("bkms")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(path, []byte("replaced"), 0o700)).To(Succeed())
		err = m.Run(
			"bkms",
			"alpha",
			nil,
			Streams{In: strings.NewReader(""), Out: io.Discard, Err: io.Discard},
		)
		expectPluginError(err, CodeDigestMismatch)
	})
})
