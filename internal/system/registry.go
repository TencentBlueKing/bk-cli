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

// Package system defines YAML-driven service/action metadata and execution helpers.
package system

// Param defines a generated CLI flag for a YAML-backed action.
type Param struct {
	Name        string `yaml:"name"`
	In          string `yaml:"in"`   // path, query, header
	Type        string `yaml:"type"` // string, bool, int
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default"`
}

// AuthConfig describes the auth and permission metadata of a YAML-defined action.
type AuthConfig struct {
	AppVerifiedRequired        bool `yaml:"appVerifiedRequired"`
	UserVerifiedRequired       bool `yaml:"userVerifiedRequired"`
	ResourcePermissionRequired bool `yaml:"resourcePermissionRequired"`
}

// RequiresAuth reports whether an action needs any generated auth header.
func (c *AuthConfig) RequiresAuth() bool {
	if c == nil {
		return true
	}
	return c.RequiresAppVerification() || c.RequiresUserVerification()
}

// RequiresAppVerification reports whether app verification fields are required.
func (c *AuthConfig) RequiresAppVerification() bool {
	if c == nil {
		return true
	}
	return c.AppVerifiedRequired
}

// RequiresUserVerification reports whether user identity fields are required.
func (c *AuthConfig) RequiresUserVerification() bool {
	if c == nil {
		return true
	}
	return c.UserVerifiedRequired
}

// Action defines an operation within a system.
type Action struct {
	Name         string      `yaml:"name"`
	Description  string      `yaml:"description"`
	Method       string      `yaml:"method"`
	Path         string      `yaml:"path"`
	Timeout      string      `yaml:"timeout,omitempty"`
	AuthConfig   *AuthConfig `yaml:"authConfig"`
	Params       []Param     `yaml:"params"`
	Examples     []string    `yaml:"examples"`
	BodySchema   string      `yaml:"body_schema,omitempty"`
	BodyRequired bool        `yaml:"body_required,omitempty"`
}

// System represents a YAML-defined system plus its actions.
type System struct {
	Name        string   `yaml:"name"`
	GatewayName string   `yaml:"gateway_name"`
	Description string   `yaml:"description"`
	Actions     []Action `yaml:"actions"`
}
