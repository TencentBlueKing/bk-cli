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

package auth_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	authcmd "github.com/TencentBlueKing/bk-cli/cmd/auth"
	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
)

// newTestRoot creates a root command wired with auth subcommands and the --context flag.
func newTestRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "bk-cli",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().String("context", "", "Override active context")
	root.AddCommand(authcmd.NewAuthCmd())
	return root
}

func executeCmd(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func executeCmdWithStderr(root *cobra.Command, args ...string) (string, string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	origStderr := os.Stderr
	r, w, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())
	os.Stderr = w

	runErr := root.Execute()

	Expect(w.Close()).To(Succeed())
	os.Stderr = origStderr

	stderr, readErr := io.ReadAll(r)
	Expect(readErr).NotTo(HaveOccurred())
	Expect(r.Close()).To(Succeed())

	return buf.String(), string(stderr), runErr
}

var _ = Describe("Auth Commands", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-auth-test-*")
		Expect(err).NotTo(HaveOccurred())
		os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)

		cfg := &config.Config{
			BkAPIURLTmpl: "https://bkapi.example.com/api/{gateway_name}/",
			UserKey:      "bk_token",
		}
		Expect(config.CreateContext("default", cfg)).To(Succeed())
		Expect(config.SetActiveContext("default")).To(Succeed())
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
		os.Unsetenv("BK_CLI_CONFIG_DIR")
	})

	Describe("login", func() {
		It("succeeds with bk_app_code + bk_app_secret + bk_token", func() {
			root := newTestRoot()
			out, err := executeCmd(root, "auth", "login",
				"--bk_app_code=myapp",
				"--bk_app_secret=mysecret",
				"--bk_token=mytoken",
			)
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())
			Expect(env["message"]).To(ContainSubstring("default"))

			// Verify credentials file was created
			credPath := config.CredentialsPath("default")
			Expect(credential.Exists(credPath)).To(BeTrue())
		})

		It("succeeds with access_token", func() {
			root := newTestRoot()
			out, err := executeCmd(root, "auth", "login",
				"--access_token=my_access_token",
			)
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			// Verify we can load and it's the right type
			credPath := config.CredentialsPath("default")
			key, err := credential.DeriveKey()
			Expect(err).NotTo(HaveOccurred())
			cred, err := credential.LoadFromFile(credPath, key)
			Expect(err).NotTo(HaveOccurred())
			Expect(cred.Type).To(Equal(credential.TypeAccessToken))
			Expect(cred.AccessToken).To(Equal("my_access_token"))
		})

		It("returns JSON error when no flags provided (device flow not implemented)", func() {
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not_implemented"))
		})

		It("returns a context error when no context is initialized", func() {
			Expect(config.DeleteContext("default")).To(Succeed())
			Expect(os.Remove(filepath.Join(tmpDir, "current"))).To(Succeed())

			root := newTestRoot()
			_, stderr, err := executeCmdWithStderr(root, "auth", "login",
				"--bk_app_code=myapp",
				"--bk_app_secret=mysecret",
				"--bk_token=mytoken",
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context init"))
			Expect(stderr).To(ContainSubstring(`--bk_api_url_tmpl=URL`))
		})

		It("fails with incomplete app credentials", func() {
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login",
				"--bk_app_code=myapp",
			)
			Expect(err).To(HaveOccurred())
		})

		It("explains that app credential mode needs a user credential when user_key is bk_token", func() {
			root := newTestRoot()
			_, stderr, err := executeCmdWithStderr(root, "auth", "login",
				"--bk_app_code=myapp",
				"--bk_app_secret=mysecret",
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("provide --bk_token or --bk_ticket"))
			Expect(err.Error()).To(ContainSubstring("current context defaults to --bk_token"))
			Expect(stderr).To(ContainSubstring(`--bk_app_code=APP_CODE`))
			Expect(stderr).To(ContainSubstring(`--bk_app_secret=APP_SECRET`))
			Expect(stderr).To(ContainSubstring(`--bk_token=VALUE`))
		})

		It("explains that app credential mode needs a user credential when user_key is bk_ticket", func() {
			cfg := &config.Config{
				BkAPIURLTmpl: "https://bkapi.example.com/api/{gateway_name}/",
				UserKey:      "bk_ticket",
			}
			Expect(cfg.Save(config.ConfigPath("default"))).To(Succeed())

			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login",
				"--bk_app_code=myapp",
				"--bk_app_secret=mysecret",
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("provide --bk_token or --bk_ticket"))
			Expect(err.Error()).To(ContainSubstring("current context defaults to --bk_ticket"))
		})

		It("stores bk_ticket credentials when provided", func() {
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login",
				"--bk_app_code=myapp",
				"--bk_app_secret=mysecret",
				"--bk_ticket=myticket",
			)
			Expect(err).NotTo(HaveOccurred())

			credPath := config.CredentialsPath("default")
			key, err := credential.DeriveKey()
			Expect(err).NotTo(HaveOccurred())
			cred, err := credential.LoadFromFile(credPath, key)
			Expect(err).NotTo(HaveOccurred())
			Expect(cred.BkTicket).To(Equal("myticket"))
			Expect(cred.BkToken).To(BeEmpty())
		})

		It("rejects tenant_id because tenant belongs to context config", func() {
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login",
				"--access_token=my_access_token",
				"--tenant_id=tenant-a",
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown flag"))
			Expect(err.Error()).To(ContainSubstring("tenant_id"))
		})
	})

	Describe("status", func() {
		It("shows credential info after login", func() {
			// First login
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login",
				"--bk_app_code=myapp123",
				"--bk_app_secret=mysecret",
				"--bk_token=mytoken",
			)
			Expect(err).NotTo(HaveOccurred())

			// Then check status
			root2 := newTestRoot()
			out, err := executeCmd(root2, "auth", "status")
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			data, ok := env["data"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(data["context"]).To(Equal("default"))
			Expect(data["credential_type"]).To(Equal("app_user"))
			Expect(data["has_credentials"]).To(BeTrue())
			Expect(data["bk_app_code"]).To(Equal("my***23"))
			Expect(data["user_key"]).To(Equal("bk_token"))
		})

		It("returns has_credentials false when no credentials exist", func() {
			root := newTestRoot()
			out, err := executeCmd(root, "auth", "status")
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			data, ok := env["data"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(data["context"]).To(Equal("default"))
			Expect(data["has_credentials"]).To(BeFalse())
			Expect(data).NotTo(HaveKey("credential_type"))
		})
	})

	Describe("check", func() {
		It("succeeds when credentials exist", func() {
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login",
				"--access_token=my_access_token",
			)
			Expect(err).NotTo(HaveOccurred())

			root2 := newTestRoot()
			out, err := executeCmd(root2, "auth", "check")
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			data, ok := env["data"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(data["context"]).To(Equal("default"))
			Expect(data["credential_type"]).To(Equal("access_token"))
			Expect(data["has_credentials"]).To(BeTrue())
		})

		It("returns error when no credentials exist", func() {
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "check")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no_credentials"))
		})
	})

	Describe("logout", func() {
		It("removes credentials after login", func() {
			// First login
			root := newTestRoot()
			_, err := executeCmd(root, "auth", "login",
				"--bk_app_code=myapp",
				"--bk_app_secret=mysecret",
				"--bk_token=mytoken",
			)
			Expect(err).NotTo(HaveOccurred())

			credPath := config.CredentialsPath("default")
			Expect(credential.Exists(credPath)).To(BeTrue())

			// Then logout
			root2 := newTestRoot()
			out, err := executeCmd(root2, "auth", "logout")
			Expect(err).NotTo(HaveOccurred())

			var env map[string]any
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env["ok"]).To(BeTrue())

			Expect(credential.Exists(credPath)).To(BeFalse())
		})
	})
})
