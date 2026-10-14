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

package aidev

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
	"github.com/TencentBlueKing/bk-cli/internal/config"
)

var _ = Describe("aidev upload", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-aidev-upload-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("sends a multipart file upload request", func() {
		type capturedRequest struct {
			method      string
			path        string
			contentType string
			authHeader  string
			spaceID     string
			module      string
			fileName    string
			fileContent string
		}

		var captured capturedRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			captured.method = r.Method
			captured.path = r.URL.Path
			captured.contentType = r.Header.Get("Content-Type")
			captured.authHeader = r.Header.Get("X-Bkapi-Authorization")

			if err := r.ParseMultipartForm(1024); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			captured.spaceID = r.FormValue("space_id")
			captured.module = r.FormValue("module")

			file, header, err := r.FormFile("file")
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			defer func() { _ = file.Close() }()
			captured.fileName = header.Filename
			content, err := io.ReadAll(file)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			captured.fileContent = string(content)

			_, _ = w.Write([]byte(`{"uploaded":true}`))
		}))
		DeferCleanup(server.Close)

		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
		filePath := filepath.Join(tmpDir, "demo.txt")
		Expect(os.WriteFile(filePath, []byte("demo"), 0o600)).To(Succeed())

		cmd := newCreatePrivateUploadCmd(systemtest.BuildDeps(false))
		cmd.SetArgs([]string{"--file", filePath, "--space_id", "demo-space", "--module", "knowledge"})

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.Execute()
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		Expect(env["ok"]).To(BeTrue())
		Expect(env["status"]).To(Equal(float64(http.StatusOK)))

		Expect(captured.method).To(Equal(http.MethodPost))
		Expect(captured.path).To(Equal("/bk-aidev/prod/openapi/aidev/private/v1/upload/"))
		Expect(captured.contentType).To(HavePrefix("multipart/form-data; boundary="))
		Expect(captured.authHeader).To(ContainSubstring(`"access_token":"token-123"`))
		Expect(captured.spaceID).To(Equal("demo-space"))
		Expect(captured.module).To(Equal("knowledge"))
		Expect(captured.fileName).To(Equal("demo.txt"))
		Expect(captured.fileContent).To(Equal("demo"))
	})

	It("redacts auth and shows multipart metadata during dry-run", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		filePath := filepath.Join(tmpDir, "demo.txt")
		Expect(os.WriteFile(filePath, []byte("demo"), 0o600)).To(Succeed())

		cmd := newCreatePrivateUploadCmd(systemtest.BuildDeps(true))
		cmd.SetArgs([]string{"--file", filePath, "--space_id", "demo-space"})

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.Execute()
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		Expect(env["dry_run"]).To(BeTrue())

		request := env["request"].(map[string]any)
		Expect(
			request["url"],
		).To(
			Equal("https://bkapi.example.com/api/bk-aidev/prod/openapi/aidev/private/v1/upload/"),
		)
		headers := request["headers"].(map[string]any)
		Expect(headers["X-Bkapi-Authorization"]).To(Equal("{...redacted...}"))
		Expect(headers["Content-Type"]).To(HavePrefix("multipart/form-data; boundary="))
		body := request["body"].(map[string]any)
		Expect(body["file"]).To(Equal("demo.txt"))
		Expect(body["space_id"]).To(Equal("demo-space"))
		Expect(body["module"]).To(Equal("skill"))
	})

	It("rejects extra positional args", func() {
		cmd := newCreatePrivateUploadCmd(systemtest.BuildDeps(true))
		cmd.SetArgs([]string{"extra"})

		err := cmd.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown command"))
	})

	It("rejects missing file before resolving runtime", func() {
		cmd := newCreatePrivateUploadCmd(systemtest.BuildDeps(true))
		cmd.SetArgs([]string{"--space_id", "demo-space"})

		err := cmd.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("file cannot be empty"))
	})

	It("rejects explicit Content-Type overrides", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		filePath := filepath.Join(tmpDir, "demo.txt")
		Expect(os.WriteFile(filePath, []byte("demo"), 0o600)).To(Succeed())

		cmd := newCreatePrivateUploadCmd(systemtest.BuildDeps(true))
		cmd.SetArgs(
			[]string{
				"--file",
				filePath,
				"--space_id",
				"demo-space",
				"--header",
				"Content-Type:application/json",
			},
		)

		err := cmd.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Content-Type"))
	})

	It("uses bkaidev when BK_TE_DOMAIN matches the legacy template", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())
		restore := config.SetBKTeDomainForTesting("te.example")
		DeferCleanup(restore)

		cfg, err := config.Load(config.ConfigPath("default"))
		Expect(err).NotTo(HaveOccurred())
		cfg.BkAPIURLTmpl = "https://{gateway_name}.apigw.te.example"
		Expect(cfg.Save(config.ConfigPath("default"))).To(Succeed())

		filePath := filepath.Join(tmpDir, "demo.txt")
		Expect(os.WriteFile(filePath, []byte("demo"), 0o600)).To(Succeed())

		cmd := newCreatePrivateUploadCmd(systemtest.BuildDeps(true))
		cmd.SetArgs([]string{"--file", filePath, "--space_id", "demo-space"})

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.Execute()
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		request := env["request"].(map[string]any)
		Expect(
			request["url"],
		).To(
			Equal("https://bkaidev.apigw.te.example/prod/openapi/aidev/private/v1/upload/"),
		)
	})
})
