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
	"io/fs"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

// validParamLocations lists the accepted values for Param.In.
var validParamLocations = map[string]bool{
	"path":   true,
	"query":  true,
	"header": true,
}

// LoadFromYAML parses a single YAML file into a System.
func LoadFromYAML(data []byte) (*System, error) {
	var sys System
	if err := yaml.Unmarshal(data, &sys); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	if sys.Name == "" {
		return nil, fmt.Errorf("system name is required in YAML definition")
	}
	if sys.GatewayName == "" {
		return nil, fmt.Errorf("gateway_name is required in YAML definition for system %q", sys.Name)
	}
	if err := validate.ValidateGatewayName(sys.GatewayName); err != nil {
		return nil, fmt.Errorf("invalid gateway_name for system %q: %w", sys.Name, err)
	}

	for _, action := range sys.Actions {
		for _, p := range action.Params {
			if p.In == "" {
				return nil, fmt.Errorf(
					"system %q action %q param %q: \"in\" is required (path, query, or header)",
					sys.Name, action.Name, p.Name,
				)
			}
			if !validParamLocations[p.In] {
				return nil, fmt.Errorf(
					"system %q action %q param %q: unsupported \"in\" value %q (must be path, query, or header)",
					sys.Name,
					action.Name,
					p.Name,
					p.In,
				)
			}
			if p.In == "path" {
				placeholder := "{" + p.Name + "}"
				if !strings.Contains(action.Path, placeholder) {
					return nil, fmt.Errorf(
						"system %q action %q param %q: in=path but no {%s} placeholder in path %q",
						sys.Name,
						action.Name,
						p.Name,
						p.Name,
						action.Path,
					)
				}
			}
		}
		if action.AuthConfig == nil {
			return nil, fmt.Errorf(
				"system %q action %q: authConfig is required",
				sys.Name,
				action.Name,
			)
		}
		if action.AuthConfig.ResourcePermissionRequired && !action.AuthConfig.AppVerifiedRequired {
			return nil, fmt.Errorf(
				"system %q action %q: authConfig.resourcePermissionRequired requires appVerifiedRequired=true",
				sys.Name,
				action.Name,
			)
		}
	}

	return &sys, nil
}

// LoadSystemFromFS loads one named YAML system definition file.
func LoadSystemFromFS(fsys fs.FS, path string) (*System, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	sys, err := LoadFromYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s: %w", filepath.Base(path), err)
	}

	return sys, nil
}

// LoadAllFromFS loads all .yaml files from an fs.FS directory.
func LoadAllFromFS(fsys fs.FS, dir string) ([]*System, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded directory %q: %w", dir, err)
	}

	var systems []*System
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		sys, err := LoadSystemFromFS(fsys, filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}

		systems = append(systems, sys)
	}

	return systems, nil
}
