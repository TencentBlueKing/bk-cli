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

package apigateway

import (
	"strings"

	json "github.com/goccy/go-json"
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newDemoActionCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var (
		name    string
		public  bool
		stage   string
		body    string
		headers []string
	)

	cmd := &cobra.Command{
		Use:   "demo_action",
		Short: "Example Go-implemented orchestration action",
		Long: `Example Go-implemented system action.

This command demonstrates the Go-owned extension path for system commands:
it performs local logic, keeps local-only flags, and then calls shared lower-layer API helpers.`,
		Example: strings.Join([]string{
			"  bk-cli apigateway demo_action --name \"demo\" --public \\",
			"    --body '{\"hello\":\"world\"}' --header 'foo:bar' --stage testing",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			received := map[string]any{
				"name":    name,
				"public":  public,
				"body":    body,
				"headers": append([]string(nil), headers...),
				"stage":   stage,
			}

			return systemcmd.ExecuteRequest(cmd, runtime, "demo_action", syslib.RequestSpec{
				GatewayName: "bk-apigateway",
				Method:      "GET",
				Path:        "/api/v2/open/gateways/",
				ParamsJSON:  buildDemoActionParamsJSON(name),
				Headers:     headers,
				Stage:       stage,
				AuthConfig: &syslib.AuthConfig{
					AppVerifiedRequired:        true,
					UserVerifiedRequired:       false,
					ResourcePermissionRequired: false,
				},
			}, func(env *output.Envelope) error {
				if env.DryRun {
					env.Data = map[string]any{
						"received": received,
					}
					return nil
				}

				env.Data = map[string]any{
					"received": received,
					"upstream": env.Data,
				}
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Gateway name filter used by the sample upstream API call")
	cmd.Flags().BoolVar(&public, "public", false, "Example local-only flag handled by Go logic")
	cmd.Flags().StringVar(
		&body,
		syslib.ActionBodyFlagName,
		"",
		"Example local-only body captured by the Go-implemented action",
	)
	systemcmd.AddCommonRequestFlagsWithoutBody(cmd, &stage, &headers)

	return cmd
}

func buildDemoActionParamsJSON(name string) string {
	if name == "" {
		return ""
	}

	data, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return ""
	}
	return string(data)
}
