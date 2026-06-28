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
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
)

var _ = Describe("doctor command", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-doctor-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("reports contexts, selected credentials, and rendered URL without leaking secrets", func() {
		Expect((&config.Config{
			BkAPIURLTmpl: "https://bkapi.example.com/api/{gateway_name}/",
			UserKey:      "bk_token",
		}).Save(config.ConfigPath("default"))).To(Succeed())
		Expect((&config.Config{
			BkAPIURLTmpl: "https://bkapi-dev.example.com/api/{gateway_name}/",
			UserKey:      "bk_ticket",
		}).Save(config.ConfigPath("dev"))).To(Succeed())
		Expect(config.SetActiveContext("default")).To(Succeed())
		saveDoctorTestCredential("default", &credential.Credential{
			Type:        credential.TypeAppUser,
			BkAppCode:   "demo-app",
			BkAppSecret: "secret-123456",
			BkToken:     "token-abcdef",
		})

		out, err := executeDoctorCmd(func() string { return "" }, "--offline")
		Expect(err).NotTo(HaveOccurred())

		data := decodeDoctorData(out)
		Expect(data["active_context"]).To(Equal("default"))
		Expect(data["selected_context"]).To(Equal("default"))

		contexts := data["contexts"].([]any)
		Expect(contexts).To(HaveLen(2))
		defaultCtx := findDoctorContext(contexts, "default")
		Expect(defaultCtx["active"]).To(BeTrue())
		Expect(defaultCtx["selected"]).To(BeTrue())
		Expect(defaultCtx["rendered_url"]).To(Equal("https://bkapi.example.com/api/bk-apigateway/prod/"))

		cred := defaultCtx["credential"].(map[string]any)
		Expect(cred["type"]).To(Equal("app_user"))
		Expect(cred["bk_app_code"]).To(Equal("de***pp"))
		Expect(cred["bk_app_secret"]).To(Equal("secr***3456"))
		Expect(cred["bk_token"]).To(Equal("toke***cdef"))
		Expect(out).NotTo(ContainSubstring("secret-123456"))
		Expect(out).NotTo(ContainSubstring("token-abcdef"))

		devCtx := findDoctorContext(contexts, "dev")
		Expect(devCtx["has_credentials"]).To(BeFalse())

		checks := data["checks"].([]any)
		Expect(findDoctorCheck(checks, "connectivity")["status"]).To(Equal("skip"))
	})

	It("checks connectivity against the rendered selected context URL", func() {
		var gotPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		}))
		DeferCleanup(server.Close)

		Expect((&config.Config{
			BkAPIURLTmpl: strings.TrimRight(server.URL, "/") + "/api/{gateway_name}/",
		}).Save(config.ConfigPath("default"))).To(Succeed())
		Expect(config.SetActiveContext("default")).To(Succeed())
		saveDoctorTestCredential("default", &credential.Credential{
			Type:        credential.TypeAccessToken,
			AccessToken: "access-token-abcdef",
		})

		out, err := executeDoctorCmd(func() string { return "" })
		Expect(err).NotTo(HaveOccurred())

		data := decodeDoctorData(out)
		Expect(data["ok"]).To(BeTrue())
		Expect(gotPath).To(Equal("/api/bk-apigateway/prod/"))
		Expect(findDoctorCheck(data["checks"].([]any), "connectivity")["status"]).To(Equal("pass"))
	})
})

func executeDoctorCmd(contextGetter func() string, args ...string) (string, error) {
	root := &cobra.Command{
		Use:           "bk-cli",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().String("context", "", "Override active context")
	doctor := newDoctorCmd(contextGetter, func() bool { return false })
	root.AddCommand(doctor)

	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	doctor.SetOut(buf)
	doctor.SetErr(buf)
	root.SetArgs(append([]string{"doctor"}, args...))

	err := root.Execute()
	return buf.String(), err
}

func saveDoctorTestCredential(ctxName string, cred *credential.Credential) {
	key, err := credential.DeriveKey()
	Expect(err).NotTo(HaveOccurred())
	Expect(credential.Save(config.CredentialsPath(ctxName), cred, key)).To(Succeed())
}

func decodeDoctorData(out string) map[string]any {
	var env map[string]any
	Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
	Expect(env["data"]).NotTo(BeNil())
	return env["data"].(map[string]any)
}

func findDoctorContext(contexts []any, name string) map[string]any {
	for _, item := range contexts {
		ctx := item.(map[string]any)
		if ctx["name"] == name {
			return ctx
		}
	}
	Fail("doctor context not found: " + name)
	return nil
}

func findDoctorCheck(checks []any, name string) map[string]any {
	for _, item := range checks {
		check := item.(map[string]any)
		if check["name"] == name {
			return check
		}
	}
	Fail("doctor check not found: " + name)
	return nil
}
