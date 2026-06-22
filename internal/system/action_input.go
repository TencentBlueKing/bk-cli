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
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

const (
	ActionStageFlagName      = "stage"
	ActionBodyFlagName       = "body"
	ActionHeaderFlagName     = "header"
	ActionBodySchemaFlagName = "body-schema"
)

var reservedGeneratedFlagNames = map[string]struct{}{
	ActionStageFlagName:      {},
	ActionBodyFlagName:       {},
	ActionHeaderFlagName:     {},
	ActionBodySchemaFlagName: {},
	"help":                   {},
	"context":                {},
	"format":                 {}, // keep it built-in reserved, would be used in the furtur
	"dry-run":                {},
	"verbose":                {},
	"insecure":               {},
}

// ActionFlag describes a generated CLI flag for a YAML param.
type ActionFlag struct {
	Param    Param
	FlagName string
}

// ActionInputSpec contains generated CLI flag metadata plus help-only header
// metadata for a YAML action.
type ActionInputSpec struct {
	GeneratedFlags []ActionFlag
	HeaderParams   []Param
}

// BuildActionInputSpec validates YAML params used for CLI flag generation and
// returns the final generated flag metadata shared by registration and runtime.
func BuildActionInputSpec(action *Action) (*ActionInputSpec, error) {
	if action == nil {
		return nil, fmt.Errorf("action is required")
	}

	spec := &ActionInputSpec{
		GeneratedFlags: make([]ActionFlag, 0, len(action.Params)),
		HeaderParams:   make([]Param, 0, len(action.Params)),
	}
	seenPath := make(map[string]struct{})
	seenQuery := make(map[string]struct{})
	seenFlags := make(map[string]struct{})

	for _, p := range action.Params {
		if p.In == "header" {
			spec.HeaderParams = append(spec.HeaderParams, p)
			continue
		}
		if p.In != "path" && p.In != "query" {
			return nil, fmt.Errorf(
				"unsupported param location %q for %q; only path and query params can "+
					"generate CLI flags (header params are help-only)",
				p.In,
				p.Name,
			)
		}

		if err := validateGeneratedParamDefault(p); err != nil {
			return nil, err
		}

		flagName := GeneratedFlagName(p)
		if _, reserved := reservedGeneratedFlagNames[flagName]; reserved {
			return nil, fmt.Errorf(
				"param name conflict %q would collide with built-in CLI flag --%s",
				p.Name,
				flagName,
			)
		}

		switch p.In {
		case "path":
			if _, exists := seenPath[p.Name]; exists {
				return nil, fmt.Errorf(
					"duplicate path param %q would create duplicate CLI flag --%s",
					p.Name,
					flagName,
				)
			}
			if _, exists := seenQuery[p.Name]; exists {
				return nil, fmt.Errorf(
					"param name conflict %q between path and query would create duplicate CLI flag --%s",
					p.Name,
					flagName,
				)
			}
			seenPath[p.Name] = struct{}{}
		case "query":
			if _, exists := seenQuery[p.Name]; exists {
				return nil, fmt.Errorf(
					"duplicate query param %q would create duplicate CLI flag --%s",
					p.Name,
					flagName,
				)
			}
			if _, exists := seenPath[p.Name]; exists {
				return nil, fmt.Errorf(
					"param name conflict %q between path and query would create duplicate CLI flag --%s",
					p.Name,
					flagName,
				)
			}
			seenQuery[p.Name] = struct{}{}
		}

		if _, exists := seenFlags[flagName]; exists {
			return nil, fmt.Errorf("generated CLI flag collision on --%s", flagName)
		}
		seenFlags[flagName] = struct{}{}

		spec.GeneratedFlags = append(spec.GeneratedFlags, ActionFlag{Param: p, FlagName: flagName})
	}

	return spec, nil
}

func validateGeneratedParamDefault(p Param) error {
	if p.Type != "int" || p.Default == "" {
		return nil
	}

	if _, err := strconv.Atoi(p.Default); err != nil {
		return fmt.Errorf(`param %q has invalid int default value %q`, p.Name, p.Default)
	}

	return nil
}

// GeneratedFlagName returns the final CLI flag name for a YAML param.
func GeneratedFlagName(p Param) string {
	return p.Name
}

// RegisterActionFlags registers generated CLI flags from a YAML action input spec
// onto the given cobra command. This is shared by production registration and test helpers.
func RegisterActionFlags(cmd *cobra.Command, inputSpec *ActionInputSpec) {
	for _, flag := range inputSpec.GeneratedFlags {
		p := flag.Param
		switch p.Type {
		case "bool":
			cmd.Flags().Bool(flag.FlagName, p.Default == "true", p.Description)
		case "int":
			defaultInt := 0
			if p.Default != "" {
				defaultInt, _ = strconv.Atoi(p.Default)
			}
			cmd.Flags().Int(flag.FlagName, defaultInt, p.Description)
		default:
			cmd.Flags().String(flag.FlagName, p.Default, p.Description)
		}
		if p.Required {
			_ = cmd.MarkFlagRequired(flag.FlagName)
		}
	}
}
