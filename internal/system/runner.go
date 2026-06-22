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

package system

import (
	"errors"
	"fmt"
	"strings"

	json "github.com/goccy/go-json"
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/api"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

// RunAction executes a YAML-defined action.
// It resolves generated path/query flags plus standard --body/--header inputs,
// builds the API request, and returns the response envelope.
func RunAction(
	action *Action,
	inputSpec *ActionInputSpec,
	gatewayName string,
	cmd *cobra.Command,
	runtime *Runtime,
	stage string,
) error {
	path := action.Path
	pathValues := make(map[string]string)
	queryMap := make(map[string]any)
	bodyJSON, _ := cmd.Flags().GetString(ActionBodyFlagName)
	headerFlags, _ := cmd.Flags().GetStringArray(ActionHeaderFlagName)

	if err := ValidateRequiredBody(action, cmd); err != nil {
		return err
	}

	for _, flag := range inputSpec.GeneratedFlags {
		val, changed, err := readFlagValue(cmd, flag)
		if err != nil {
			return err
		}

		switch flag.Param.In {
		case "path":
			pathValues[flag.Param.Name] = fmt.Sprintf("%v", val)
		case "query":
			if shouldIncludeQueryParam(flag.Param, val, changed) {
				queryMap[flag.Param.Name] = val
			}
		}
	}

	path, err := api.SubstitutePathValues(path, pathValues, map[string]api.PathValueValidator{
		"gateway_name": validate.ValidateGatewayName,
	})
	if err != nil {
		var fieldErr *validate.FieldError
		if errors.As(err, &fieldErr) && fieldErr.Field == "gateway_name" {
			return output.UserError(
				"invalid_gateway_name",
				err.Error(),
				"Use a gateway name matching ^[a-z][a-z0-9-]{2,29}$",
			)
		}
		return output.UserError(
			"path_error",
			err.Error(),
			"Check that all path params are defined and provided",
		)
	}

	var paramsJSON string
	if len(queryMap) > 0 {
		data, err := json.Marshal(queryMap)
		if err != nil {
			return output.SystemError("param_error", fmt.Sprintf("failed to marshal params: %s", err), "")
		}
		paramsJSON = string(data)
	}

	result, err := ExecuteRequest(runtime, RequestSpec{
		GatewayName: gatewayName,
		Method:      strings.ToUpper(action.Method),
		Path:        path,
		ParamsJSON:  paramsJSON,
		BodyJSON:    bodyJSON,
		Headers:     headerFlags,
		Stage:       stage,
		Timeout:     action.Timeout,
		AuthConfig:  action.AuthConfig,
	})
	if err != nil {
		return err
	}

	return result.Envelope.WriteJSON(cmd.OutOrStdout())
}

// ValidateRequiredBody returns a local input error before runtime resolution
// when a YAML action requires an explicit JSON body.
func ValidateRequiredBody(action *Action, cmd *cobra.Command) error {
	bodyJSON, _ := cmd.Flags().GetString(ActionBodyFlagName)
	if !action.BodyRequired || strings.TrimSpace(bodyJSON) != "" {
		return nil
	}

	return output.UserError(
		"missing_param",
		fmt.Sprintf("required parameter --%s is missing", ActionBodyFlagName),
		requiredParamUsage(cmd, ActionBodyFlagName),
	)
}

// readFlagValue reads a param's value from cobra flags. Returns the value,
// whether the flag was explicitly changed, and any validation error.
func readFlagValue(cmd *cobra.Command, flag ActionFlag) (any, bool, error) {
	p := flag.Param
	flagName := flag.FlagName
	changed := cmd.Flags().Changed(flagName)
	switch p.Type {
	case "bool":
		val, _ := cmd.Flags().GetBool(flagName)
		return val, changed, nil
	case "int":
		val, _ := cmd.Flags().GetInt(flagName)
		if val == 0 && p.Required && !changed {
			return nil, false, output.UserError(
				"missing_param",
				fmt.Sprintf("required parameter --%s is missing", flagName),
				requiredParamUsage(cmd, flagName),
			)
		}
		return val, changed, nil
	default:
		val, _ := cmd.Flags().GetString(flagName)
		if val == "" && p.Required {
			return nil, false, output.UserError(
				"missing_param",
				fmt.Sprintf("required parameter --%s is missing", flagName),
				requiredParamUsage(cmd, flagName),
			)
		}
		return val, changed, nil
	}
}

func requiredParamUsage(cmd *cobra.Command, flagName string) string {
	commandPath := cmd.CommandPath()
	if commandPath == "" {
		commandPath = cmd.Name()
	}
	if commandPath != "bk-cli" && !strings.HasPrefix(commandPath, "bk-cli ") {
		commandPath = "bk-cli " + commandPath
	}
	return fmt.Sprintf("Usage: %s --%s=VALUE", commandPath, flagName)
}

// shouldIncludeQueryParam determines whether a query parameter should be
// included based on its type, value, and whether the flag was changed.
func shouldIncludeQueryParam(p Param, val any, changed bool) bool {
	switch p.Type {
	case "bool":
		b, _ := val.(bool)
		return b || changed
	case "int":
		return changed || p.Required
	default:
		s, _ := val.(string)
		return s != ""
	}
}
