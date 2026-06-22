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

// Package testutil provides shared helpers for cmd/system action tests.
package testutil

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

// CaptureCommandStdout captures stdout produced while fn runs and returns fn's error unchanged.
func CaptureCommandStdout(fn func() error) (string, error) {
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
	if readErr != nil {
		return "", readErr
	}
	return string(out), runErr
}

// SetupTestContext creates a default access-token context suitable for command tests.
func SetupTestContext(baseURL string) error {
	cfg := &config.Config{
		BkAPIURLTmpl: strings.TrimRight(baseURL, "/") + "/{gateway_name}/",
		UserKey:      "bk_token",
	}
	if err := cfg.Save(config.ConfigPath("default")); err != nil {
		return err
	}
	if err := config.SetActiveContext("default"); err != nil {
		return err
	}

	key, err := credential.DeriveKey()
	if err != nil {
		return err
	}
	cred := &credential.Credential{Type: credential.TypeAccessToken, AccessToken: "token-123"}
	return credential.Save(config.CredentialsPath("default"), cred, key)
}

// BuildDeps creates the common BuildDeps used by Go-implemented system action tests.
func BuildDeps(dryRun bool) systemcmd.BuildDeps {
	return systemcmd.BuildDeps{
		GetContext: func() string { return "" },
		IsDryRun:   func() bool { return dryRun },
		IsVerbose:  func() bool { return false },
	}
}

// BuildYAMLActionCmd constructs a YAML-backed action command for tests.
func BuildYAMLActionCmd(
	yamlFS fs.FS,
	yamlPath string,
	actionName string,
	deps systemcmd.BuildDeps,
) (*cobra.Command, error) {
	sys, err := syslib.LoadSystemFromFS(yamlFS, yamlPath)
	if err != nil {
		return nil, err
	}

	var action *syslib.Action
	for i := range sys.Actions {
		if sys.Actions[i].Name == actionName {
			action = &sys.Actions[i]
			break
		}
	}
	if action == nil {
		return nil, fmt.Errorf("action %q not found in %s", actionName, yamlPath)
	}

	inputSpec, err := syslib.BuildActionInputSpec(action)
	if err != nil {
		return nil, err
	}

	stage := "prod"
	body := ""
	headers := []string{}

	cmd := &cobra.Command{
		Use: action.Name,
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			return syslib.RunAction(action, inputSpec, sys.GatewayName, cmd, runtime, stage)
		},
	}
	parent := &cobra.Command{Use: sys.Name}
	parent.AddCommand(cmd)

	cmd.Flags().StringVar(&stage, syslib.ActionStageFlagName, "prod", "[Optional] API gateway stage")
	cmd.Flags().StringVar(&body, syslib.ActionBodyFlagName, "", "[Optional] JSON request body")
	cmd.Flags().StringArrayVar(
		&headers,
		syslib.ActionHeaderFlagName,
		nil,
		"[Optional] Additional headers (key:value, repeatable; auth/tenant overrides allowed)",
	)

	syslib.RegisterActionFlags(cmd, inputSpec)

	return cmd, nil
}
