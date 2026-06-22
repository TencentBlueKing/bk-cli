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

package context_test

import (
	"io"
	"os"
	"path/filepath"
	"time"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctxcmd "github.com/TencentBlueKing/bk-cli/cmd/context"
	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

// executeCmd runs the context command tree with the given args,
// capturing real stdout (where envelope.Print writes).
func executeCmd(args ...string) (string, error) {
	// Capture os.Stdout
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := ctxcmd.NewContextCmd()
	cmd.PersistentFlags().String("context", "", "Override active context")
	cmd.SetArgs(args)
	err := cmd.Execute()

	w.Close()
	os.Stdout = origStdout

	out, _ := io.ReadAll(r)
	r.Close()

	return string(out), err
}

func executeCmdWithStderr(args ...string) (string, string, error) {
	origStdout := os.Stdout
	origStderr := os.Stderr

	stdoutR, stdoutW, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())
	stderrR, stderrW, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())

	os.Stdout = stdoutW
	os.Stderr = stderrW

	cmd := ctxcmd.NewContextCmd()
	cmd.PersistentFlags().String("context", "", "Override active context")
	cmd.SetArgs(args)
	runErr := cmd.Execute()

	Expect(stdoutW.Close()).To(Succeed())
	Expect(stderrW.Close()).To(Succeed())
	os.Stdout = origStdout
	os.Stderr = origStderr

	stdout, readErr := io.ReadAll(stdoutR)
	Expect(readErr).NotTo(HaveOccurred())
	Expect(stdoutR.Close()).To(Succeed())

	stderr, readErr := io.ReadAll(stderrR)
	Expect(readErr).NotTo(HaveOccurred())
	Expect(stderrR.Close()).To(Succeed())

	return string(stdout), string(stderr), runErr
}

var _ = Describe("Context Commands", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-ctxcmd-*")
		Expect(err).NotTo(HaveOccurred())
		os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)
		DeferCleanup(func() {
			os.Unsetenv("BK_CLI_CONFIG_DIR")
			os.RemoveAll(tmpDir)
		})
	})

	Describe("help", func() {
		It("documents duration examples for timeout flags", func() {
			initHelp, err := executeCmd("init", "--help")
			Expect(err).NotTo(HaveOccurred())
			Expect(initHelp).To(ContainSubstring("e.g. 30s, 1m, 2m30s"))

			createHelp, err := executeCmd("create", "--help")
			Expect(err).NotTo(HaveOccurred())
			Expect(createHelp).To(ContainSubstring("e.g. 30s, 1m, 2m30s"))
		})
	})

	Describe("create", func() {
		It("initializes the default context via init", func() {
			out, err := executeCmd(
				"init",
				"--bk_api_url_tmpl=https://bkapi.example.com/api/{gateway_name}/",
			)
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())
			Expect(config.ContextExists("default")).To(BeTrue())

			active, err := config.ActiveContextName()
			Expect(err).NotTo(HaveOccurred())
			Expect(active).To(Equal("default"))
		})

		It("succeeds with a valid URL template", func() {
			out, err := executeCmd(
				"create",
				"myctx",
				"--bk_api_url_tmpl=https://bkapi.example.com/api/{gateway_name}/",
			)
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())
			Expect(env["message"]).To(Equal("Context \"myctx\" created"))

			// Verify context was created without changing the active context.
			Expect(config.ContextExists("myctx")).To(BeTrue())
			active, err := config.ActiveContextName()
			Expect(err).NotTo(HaveOccurred())
			Expect(active).To(BeEmpty())
		})

		It("keeps the current active context unchanged", func() {
			_, err := executeCmd(
				"init",
				"--bk_api_url_tmpl=https://default.example.com/api/{gateway_name}/",
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = executeCmd(
				"create",
				"myctx",
				"--bk_api_url_tmpl=https://bkapi.example.com/api/{gateway_name}/",
			)
			Expect(err).NotTo(HaveOccurred())

			active, err := config.ActiveContextName()
			Expect(err).NotTo(HaveOccurred())
			Expect(active).To(Equal("default"))
		})

		It("fails with an invalid URL template (no scheme/host)", func() {
			_, err := executeCmd("create", "bad", "--bk_api_url_tmpl=not-a-url/{gateway_name}/")
			Expect(err).To(HaveOccurred())
		})

		It("fails when {gateway_name} placeholder is missing", func() {
			_, err := executeCmd("create", "bad2", "--bk_api_url_tmpl=https://bkapi.example.com/api/fixed/")
			Expect(err).To(HaveOccurred())
		})

		It("fails for invalid context names", func() {
			out, err := executeCmd(
				"create",
				"../escape",
				"--bk_api_url_tmpl=https://bkapi.example.com/api/{gateway_name}/",
			)
			Expect(out).To(BeEmpty())
			Expect(err).To(HaveOccurred())

			cliErr, ok := err.(*output.CLIError)
			Expect(ok).To(BeTrue())
			Expect(cliErr.Code).To(Equal("invalid_context_name"))
		})

		It("normalizes legacy {api_name} placeholder on create", func() {
			_, err := executeCmd(
				"create",
				"legacy",
				"--bk_api_url_tmpl=https://bkapi.example.com/api/{api_name}/",
			)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := config.Load(config.ConfigPath("legacy"))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.BkAPIURLTmpl).To(Equal("https://bkapi.example.com/api/{gateway_name}/"))
		})

		It("normalizes legacy {api_name} placeholder on init", func() {
			_, err := executeCmd(
				"init",
				"--bk_api_url_tmpl=https://bkapi.example.com/api/{api_name}/",
			)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := config.Load(config.ConfigPath("default"))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.BkAPIURLTmpl).To(Equal("https://bkapi.example.com/api/{gateway_name}/"))
		})

		It("creates context with optional flags", func() {
			_, err := executeCmd("create", "full",
				"--bk_api_url_tmpl=https://bkapi.example.com/api/{gateway_name}/",
				"--bk_auth_url=https://auth.example.com",
				"--tenant_id=tenant1",
				"--user_key=bk_ticket",
				"--timeout=90s",
			)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := config.Load(config.ConfigPath("full"))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.BkAuthURL).To(Equal("https://auth.example.com"))
			Expect(cfg.TenantID).To(Equal("tenant1"))
			Expect(cfg.UserKey).To(Equal("bk_ticket"))
			Expect(cfg.Timeout).To(Equal(90 * time.Second))
		})

		It("defaults context timeout to 60s when not specified", func() {
			_, err := executeCmd(
				"create",
				"default-timeout",
				"--bk_api_url_tmpl=https://bkapi.example.com/api/{gateway_name}/",
			)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := config.Load(config.ConfigPath("default-timeout"))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Timeout).To(Equal(config.DefaultTimeout))
		})
	})

	Describe("list", func() {
		It("shows created contexts", func() {
			// Create two contexts
			_, err := executeCmd(
				"create",
				"alpha",
				"--bk_api_url_tmpl=https://alpha.example.com/api/{gateway_name}/",
			)
			Expect(err).NotTo(HaveOccurred())
			_, err = executeCmd(
				"create",
				"beta",
				"--bk_api_url_tmpl=https://beta.example.com/api/{gateway_name}/",
			)
			Expect(err).NotTo(HaveOccurred())

			out, err := executeCmd("list")
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			data := env["data"].(map[string]any)
			Expect(data["active"]).To(Equal(""))
			contexts := data["contexts"].([]any)
			Expect(contexts).To(HaveLen(2))
		})

		It("shows the full saved config for each context", func() {
			_, err := executeCmd(
				"create",
				"full",
				"--bk_api_url_tmpl=https://bkapi.example.com/api/{gateway_name}/",
				"--bk_auth_url=https://auth.example.com",
				"--tenant_id=tenant1",
				"--user_key=bk_ticket",
				"--timeout=90s",
			)
			Expect(err).NotTo(HaveOccurred())

			out, err := executeCmd("list")
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			contexts := env["data"].(map[string]any)["contexts"].([]any)

			var full map[string]any
			for _, item := range contexts {
				contextInfo := item.(map[string]any)
				if contextInfo["name"] == "full" {
					full = contextInfo
					break
				}
			}

			Expect(full).NotTo(BeNil())
			Expect(full["bk_api_url_tmpl"]).To(Equal("https://bkapi.example.com/api/{gateway_name}/"))
			Expect(full["bk_auth_url"]).To(Equal("https://auth.example.com"))
			Expect(full["tenant_id"]).To(Equal("tenant1"))
			Expect(full["user_key"]).To(Equal("bk_ticket"))
			Expect(full["timeout"]).To(Equal((90 * time.Second).String()))
		})
	})

	Describe("status", func() {
		It("shows the active context configuration", func() {
			_, err := executeCmd(
				"init",
				"--bk_api_url_tmpl=https://default.example.com/api/{gateway_name}/",
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = executeCmd(
				"create",
				"full",
				"--bk_api_url_tmpl=https://bkapi.example.com/api/{gateway_name}/",
				"--bk_auth_url=https://auth.example.com",
				"--tenant_id=tenant1",
				"--user_key=bk_ticket",
				"--timeout=90s",
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = executeCmd("use", "full")
			Expect(err).NotTo(HaveOccurred())

			out, err := executeCmd("status")
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			data := env["data"].(map[string]any)
			Expect(data["context"]).To(Equal("full"))
			Expect(data["bk_api_url_tmpl"]).To(Equal("https://bkapi.example.com/api/{gateway_name}/"))
			Expect(data["bk_auth_url"]).To(Equal("https://auth.example.com"))
			Expect(data["tenant_id"]).To(Equal("tenant1"))
			Expect(data["user_key"]).To(Equal("bk_ticket"))
			Expect(data["timeout"]).To(Equal((90 * time.Second).String()))
			Expect(data).NotTo(HaveKey("version"))
		})

		It("matches list formatting for defaulted and empty config fields", func() {
			_, err := executeCmd(
				"init",
				"--bk_api_url_tmpl=https://default.example.com/api/{gateway_name}/",
			)
			Expect(err).NotTo(HaveOccurred())

			out, err := executeCmd("status")
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			data := env["data"].(map[string]any)
			Expect(data["context"]).To(Equal("default"))
			Expect(data["bk_api_url_tmpl"]).To(Equal("https://default.example.com/api/{gateway_name}/"))
			Expect(data["user_key"]).To(Equal("bk_token"))
			Expect(data["timeout"]).To(Equal(config.DefaultTimeout.String()))
			Expect(data).NotTo(HaveKey("tenant_id"))
			Expect(data).NotTo(HaveKey("bk_auth_url"))
		})

		It("shows no active context when none exists", func() {
			out, err := executeCmd("status")
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			data := env["data"].(map[string]any)
			Expect(data["context"]).To(Equal("(none)"))
			Expect(data).NotTo(HaveKey("bk_api_url_tmpl"))
		})

		It("returns an error when an explicit context does not exist", func() {
			out, err := executeCmd("status", "--context", "missing")
			Expect(out).To(BeEmpty())
			Expect(err).To(HaveOccurred())

			cliErr, ok := err.(*output.CLIError)
			Expect(ok).To(BeTrue())
			Expect(cliErr.Code).To(Equal("context_error"))
		})
	})

	Describe("use", func() {
		It("switches the active context", func() {
			_, err := executeCmd(
				"create",
				"first",
				"--bk_api_url_tmpl=https://first.example.com/api/{gateway_name}/",
			)
			Expect(err).NotTo(HaveOccurred())
			_, err = executeCmd(
				"create",
				"second",
				"--bk_api_url_tmpl=https://second.example.com/api/{gateway_name}/",
			)
			Expect(err).NotTo(HaveOccurred())

			// Creating contexts does not switch the active marker.
			active, _ := config.ActiveContextName()
			Expect(active).To(BeEmpty())

			// Switch to first
			out, err := executeCmd("use", "first")
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			active, _ = config.ActiveContextName()
			Expect(active).To(Equal("first"))
		})

		It("fails for non-existent context", func() {
			_, err := executeCmd("use", "nonexistent")
			Expect(err).To(HaveOccurred())
		})

		It("fails for invalid context names", func() {
			out, err := executeCmd("use", "../escape")
			Expect(out).To(BeEmpty())
			Expect(err).To(HaveOccurred())

			cliErr, ok := err.(*output.CLIError)
			Expect(ok).To(BeTrue())
			Expect(cliErr.Code).To(Equal("invalid_context_name"))
		})
	})

	Describe("delete", func() {
		It("removes a non-active context", func() {
			_, err := executeCmd(
				"create",
				"keep",
				"--bk_api_url_tmpl=https://keep.example.com/api/{gateway_name}/",
			)
			Expect(err).NotTo(HaveOccurred())
			_, err = executeCmd(
				"create",
				"remove-me",
				"--bk_api_url_tmpl=https://rm.example.com/api/{gateway_name}/",
			)
			Expect(err).NotTo(HaveOccurred())

			// Active is "remove-me", switch to "keep" first
			_, err = executeCmd("use", "keep")
			Expect(err).NotTo(HaveOccurred())

			out, err := executeCmd("delete", "remove-me")
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())
			Expect(config.ContextExists("remove-me")).To(BeFalse())
		})

		It("fails when deleting the active context", func() {
			_, err := executeCmd(
				"create",
				"active-ctx",
				"--bk_api_url_tmpl=https://a.example.com/api/{gateway_name}/",
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = executeCmd("use", "active-ctx")
			Expect(err).NotTo(HaveOccurred())

			_, stderr, err := executeCmdWithStderr("delete", "active-ctx")
			Expect(err).To(HaveOccurred())
			Expect(stderr).To(ContainSubstring(`bk-cli context use OTHER-CONTEXT`))
			// Context should still exist
			Expect(config.ContextExists("active-ctx")).To(BeTrue())
		})

		It("fails for invalid context names", func() {
			out, err := executeCmd("delete", "../escape")
			Expect(out).To(BeEmpty())
			Expect(err).To(HaveOccurred())

			cliErr, ok := err.(*output.CLIError)
			Expect(ok).To(BeTrue())
			Expect(cliErr.Code).To(Equal("invalid_context_name"))
		})
	})

	Describe("list", func() {
		It("fails closed when a context config is malformed", func() {
			_, err := executeCmd(
				"create",
				"good",
				"--bk_api_url_tmpl=https://good.example.com/api/{gateway_name}/",
			)
			Expect(err).NotTo(HaveOccurred())

			badDir := filepath.Join(tmpDir, "contexts", "bad")
			Expect(os.MkdirAll(badDir, 0o700)).To(Succeed())
			Expect(
				os.WriteFile(
					filepath.Join(badDir, "config.yaml"),
					[]byte("bk_api_url_tmpl: [broken\n"),
					0o600,
				),
			).To(Succeed())

			out, err := executeCmd("list")
			Expect(out).To(BeEmpty())
			Expect(err).To(HaveOccurred())

			cliErr, ok := err.(*output.CLIError)
			Expect(ok).To(BeTrue())
			Expect(cliErr.Code).To(Equal("invalid_context_config"))
			Expect(cliErr.Message).To(ContainSubstring("bad"))
		})
	})
})
