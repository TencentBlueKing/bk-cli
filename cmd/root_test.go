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
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func captureStdoutForRoot(fn func() error) (string, error) {
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w

	runErr := fn()

	_ = w.Close()
	os.Stdout = origStdout

	out, readErr := io.ReadAll(r)
	_ = r.Close()
	Expect(readErr).NotTo(HaveOccurred())
	return string(out), runErr
}

var _ = Describe("version command", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-root-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
		SetBuildInfo(BuildInfo{})
		rootContext = ""
		rootDryRun = false
		rootVerbose = false
		rootInsecure = false
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
		SetBuildInfo(BuildInfo{})
		rootContext = ""
		rootDryRun = false
		rootVerbose = false
		rootInsecure = false
	})

	It("renders only build metadata", func() {
		stdout, err := captureStdoutForRoot(func() error {
			return newVersionCmd().RunE(newVersionCmd(), nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())

		data := env["data"].(map[string]any)
		Expect(data["version"]).To(Equal("dev"))
		Expect(data["commit_id"]).To(Equal("unknown"))
		Expect(data["build_time"]).To(Equal("unknown"))
		Expect(data).NotTo(HaveKey("context"))
		Expect(data).NotTo(HaveKey("bk_api_url_tmpl"))
	})

	It("ignores a missing context override", func() {
		rootContext = "missing"

		stdout, err := captureStdoutForRoot(func() error {
			return newVersionCmd().RunE(newVersionCmd(), nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())
		data := env["data"].(map[string]any)
		Expect(data["version"]).To(Equal("dev"))
		Expect(data["commit_id"]).To(Equal("unknown"))
		Expect(data["build_time"]).To(Equal("unknown"))
		Expect(data).NotTo(HaveKey("context"))
	})

	It("does not expose a format flag", func() {
		cmd := newRootCmd()
		flag := cmd.PersistentFlags().Lookup("format")
		Expect(flag).To(BeNil())
	})

	It("exposes an insecure flag for HTTPS certificate verification overrides", func() {
		cmd := newRootCmd()
		flag := cmd.PersistentFlags().Lookup("insecure")
		Expect(flag).NotTo(BeNil())
		Expect(flag.Usage).To(ContainSubstring("TLS certificate verification"))
	})

	It("rejects the removed format flag", func() {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"version", "--format", "text"})

		err := cmd.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown flag: --format"))
	})

	It("labels top-level commands as root or system commands in help output", func() {
		cmd := newRootCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(out)

		err := cmd.Help()
		Expect(err).NotTo(HaveOccurred())
		Expect(
			out.String(),
		).To(
			ContainSubstring("api         [root] Make raw API calls to BlueKing API gateways"),
		)
		Expect(out.String()).To(
			ContainSubstring(
				"completion  [root] Generate the autocompletion script for the specified shell",
			),
		)
		Expect(out.String()).To(ContainSubstring("help        [root] Help about any command"))
		Expect(out.String()).To(
			ContainSubstring("skills      [root] Read embedded bk-cli skill content (list / read)"),
		)
		Expect(out.String()).To(
			ContainSubstring(
				"apigateway  [system] BlueKing API Gateway management - discover gateways and APIs",
			),
		)
		Expect(out.String()).To(
			ContainSubstring("API gateway 403: if X-Bkapi-Error-Code is 1640301"),
		)
		Expect(out.String()).To(
			ContainSubstring("bk_error_code 9900403 (IAM permission error)"),
		)
	})

	DescribeTable(
		"help examples preserve literal values in Bash",
		func(args []string, exampleValue string) {
			cmd := newRootCmd()
			out := &bytes.Buffer{}
			cmd.SetOut(out)
			cmd.SetErr(out)
			Expect(executeRoot(cmd, append(args, "-h"))).To(Succeed())

			// Replace a value inside the example's existing quotes, as a user would.
			// Synthetic shell expressions must reach the CLI literally.
			literal := exampleValue + `$BK_CLI_HELP_UNSET${BK_CLI_HELP_UNSET}$(printf expanded)` +
				"`printf expanded`" + `!"\end`
			checked := 0
			for _, line := range strings.Split(strings.ReplaceAll(out.String(), "\\\n", ""), "\n") {
				line = strings.TrimSpace(line)
				if !strings.HasPrefix(line, "bk-cli ") ||
					(!strings.Contains(line, "'"+exampleValue+"'") && !strings.Contains(line, `"`+exampleValue+`"`)) {
					continue
				}
				for _, quote := range []string{"'", `"`} {
					line = strings.ReplaceAll(line, quote+exampleValue+quote, quote+literal+quote)
				}
				// Capture arguments only; never execute requests from help examples.
				shell := exec.Command("bash", "--noprofile", "--norc", "-c",
					"unset BK_CLI_HELP_UNSET\nbk-cli() { printf '%s\\0' \"$@\"; }\n"+line)
				stdout, err := shell.Output()
				Expect(err).NotTo(HaveOccurred(), line)
				Expect(string(stdout)).To(ContainSubstring(literal+"\x00"), line)
				checked++
			}
			Expect(checked).To(BeNumerically(">", 0))
		},
		Entry("root URL template", []string{}, "https://bkapi.example.com/api/{gateway_name}/"),
		Entry("root app secret", []string{}, "secret"),
		Entry("context URL template", []string{"context"}, "https://bkapi.example.com/api/{gateway_name}/"),
		Entry(
			"context init URL template",
			[]string{"context", "init"},
			"https://bkapi.example.com/api/{gateway_name}/",
		),
		Entry(
			"context create URL template",
			[]string{"context", "create"},
			"https://bkapi.clouds.example.com/api/{gateway_name}/",
		),
		Entry("auth URL template", []string{"auth"}, "https://bkapi.example.com/api/{gateway_name}/"),
		Entry("auth app secret", []string{"auth"}, "secret"),
		Entry("auth access token", []string{"auth"}, "my_token"),
		Entry("auth login app secret", []string{"auth", "login"}, "secret"),
		Entry("auth login user token", []string{"auth", "login"}, "tok"),
		Entry("auth login access token", []string{"auth", "login"}, "my_token"),
		Entry("api header", []string{"api"}, "X-Custom:value"),
		Entry("job script content", []string{"job", "fast_execute_script"}, "echo hello"),
	)

	It("rejects unknown system action names before help handling", func() {
		cmd := newRootCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(out)

		err := executeRoot(cmd, []string{"sops", "start_taks", "-h"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`unknown command "start_taks" for "bk-cli sops"`))
		Expect(out.String()).NotTo(ContainSubstring("Available Commands:"))
	})

	It("keeps valid system action help available", func() {
		cmd := newRootCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(out)

		err := executeRoot(cmd, []string{"sops", "start_task", "-h"})
		Expect(err).NotTo(HaveOccurred())
		Expect(out.String()).To(ContainSubstring("Start executing a created task"))
		Expect(out.String()).To(ContainSubstring("--task_id int"))
	})

	It("allows separated explicit bool values for system action flags", func() {
		args := []string{
			"devops",
			"pipeline",
			"get_build_list",
			"--projectId",
			"demo",
			"--archiveFlag",
			"true",
		}

		cmd := newRootCmd()
		normalizedArgs := normalizeSystemCommandBoolArgs(cmd, args)
		Expect(normalizedArgs).To(Equal([]string{
			"devops",
			"pipeline",
			"get_build_list",
			"--projectId",
			"demo",
			"--archiveFlag=true",
		}))

		Expect(validateSystemCommandArgs(newRootCmd(), normalizedArgs)).To(Succeed())
	})

	It("still rejects extra args after separated explicit bool values", func() {
		args := []string{
			"devops",
			"pipeline",
			"get_build_list",
			"--projectId",
			"demo",
			"--archiveFlag",
			"true",
			"extra",
		}

		normalizedArgs := normalizeSystemCommandBoolArgs(newRootCmd(), args)
		err := validateSystemCommandArgs(newRootCmd(), normalizedArgs)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(
			`unknown command "extra" for "bk-cli devops pipeline get_build_list"`,
		))
	})
})
