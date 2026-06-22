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

package systemcmd

import (
	"bytes"
	"errors"
	"os"
	"strings"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
)

func writeSystemcmdContext(name, baseURL string) {
	cfg := &config.Config{BkAPIURLTmpl: strings.TrimRight(baseURL, "/") + "/{gateway_name}/"}
	Expect(cfg.Save(config.ConfigPath(name))).To(Succeed())
	Expect(config.SetActiveContext(name)).To(Succeed())
}

var _ = Describe("systemcmd runtime and execution helpers", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-systemcmd-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("resolves runtime flags from build deps", func() {
		writeSystemcmdContext("default", "https://bkapi.example.com/api")

		runtime, err := ResolveRuntime(BuildDeps{
			GetContext: func() string { return "" },
			IsDryRun:   func() bool { return true },
			IsVerbose:  func() bool { return true },
			IsInsecure: func() bool { return true },
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(runtime.DryRun).To(BeTrue())
		Expect(runtime.Verbose).To(BeTrue())
		Expect(runtime.Insecure).To(BeTrue())
		Expect(runtime.ContextName).To(Equal("default"))
	})

	It("writes the final envelope to the command output", func() {
		writeSystemcmdContext("default", "https://bkapi.example.com/api")

		runtime, err := ResolveRuntime(BuildDeps{
			IsDryRun: func() bool { return true },
		})
		Expect(err).NotTo(HaveOccurred())

		cmd := &cobra.Command{Use: "demo"}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		err = ExecuteRequest(cmd, runtime, "demo_action", syslib.RequestSpec{
			GatewayName: "bk-apigateway",
			Method:      "GET",
			Path:        "/api/v1/demo/",
			AuthConfig:  &syslib.AuthConfig{},
		}, func(env *output.Envelope) error {
			env.Message = "mutated"
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		var parsed map[string]any
		Expect(json.Unmarshal(buf.Bytes(), &parsed)).To(Succeed())
		Expect(parsed["dry_run"]).To(BeTrue())
		Expect(parsed["message"]).To(Equal("mutated"))
	})

	It("returns mutate errors without writing output", func() {
		writeSystemcmdContext("default", "https://bkapi.example.com/api")

		runtime, err := ResolveRuntime(BuildDeps{
			IsDryRun: func() bool { return true },
		})
		Expect(err).NotTo(HaveOccurred())

		cmd := &cobra.Command{Use: "demo"}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		err = ExecuteRequest(cmd, runtime, "demo_action", syslib.RequestSpec{
			GatewayName: "bk-apigateway",
			Method:      "GET",
			Path:        "/api/v1/demo/",
			AuthConfig:  &syslib.AuthConfig{},
		}, func(env *output.Envelope) error {
			return errors.New("mutate failed")
		})
		Expect(err).To(MatchError("mutate failed"))
		Expect(buf.String()).To(BeEmpty())
	})

	It("rejects invalid JSON object flags", func() {
		_, err := ParseJSONObjectFlag("target_server", `{bad`)
		Expect(err).To(MatchError("invalid_argument: target_server must be a valid JSON object"))
	})
})
