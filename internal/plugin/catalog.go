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

package plugin

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
	"golang.org/x/mod/semver"

	"github.com/TencentBlueKing/bk-cli/internal/validate"
)

//go:embed catalog.yaml
var catalogBytes []byte

// SupportedPlatforms lists the platform keys accepted in the catalog.
var SupportedPlatforms = []string{
	"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64", "windows-amd64", "windows-arm64",
}

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Catalog is the top-level plugin catalog document.
type Catalog struct {
	SchemaVersion int                   `yaml:"schema_version"`
	Plugins       map[string]Definition `yaml:"plugins"`
}

// Definition describes a plugin entry in the catalog.
type Definition struct {
	Binary             string             `yaml:"binary"`
	Description        string             `yaml:"description"`
	RecommendedVersion string             `yaml:"recommended_version"`
	Versions           map[string]Release `yaml:"versions"`
}

// Release describes a single plugin version entry.
type Release struct {
	Status    string           `yaml:"status"`
	Auth      string           `yaml:"auth,omitempty"`
	Platforms map[string]Asset `yaml:"platforms,omitempty"`
}

// Asset describes a downloadable release artifact for one platform.
type Asset struct {
	URL              string `yaml:"url"`
	ArchiveSHA256    string `yaml:"archive_sha256"`
	Executable       string `yaml:"executable"`
	ExecutableSHA256 string `yaml:"executable_sha256"`
}

// Resolved is a fully resolved plugin release asset.
type Resolved struct {
	Name, Version, Platform string
	Definition              Definition
	Release                 Release
	Asset                   Asset
}

// LoadCatalog parses the catalog embedded at build time. It is the only authorization source.
func LoadCatalog() (*Catalog, error) { return ParseCatalog(catalogBytes) }

// ArchiveFormat returns the release archive format for a platform key.
func ArchiveFormat(platform string) string {
	if strings.HasPrefix(platform, "windows-") {
		return "zip"
	}
	return "tar.gz"
}

func invalidCatalog(format string, args ...any) *Error {
	return SystemError(CodeCatalogInvalid, fmt.Sprintf(format, args...), "")
}

// ParseCatalog strictly decodes and validates catalog YAML.
func ParseCatalog(data []byte) (*Catalog, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var c Catalog
	if err := dec.Decode(&c); err != nil {
		return nil, invalidCatalog("decode catalog: %v", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, invalidCatalog("catalog must contain exactly one YAML document")
	}
	if c.SchemaVersion != 1 {
		return nil, invalidCatalog("unsupported schema_version %d", c.SchemaVersion)
	}
	for name, def := range c.Plugins {
		if err := validateDefinition(name, &def); err != nil {
			return nil, err
		}
		c.Plugins[name] = def
	}
	return &c, nil
}

func validateDefinition(name string, def *Definition) error {
	if err := validate.ValidateContextName(name); err != nil {
		return invalidCatalog("plugin %q: invalid name: %v", name, err)
	}
	if def.Binary == "" || strings.ContainsAny(def.Binary, `/\`) {
		return invalidCatalog("plugin %q: invalid binary", name)
	}
	for version, rel := range def.Versions {
		if !semver.IsValid(version) || semver.Canonical(version) != version {
			return invalidCatalog(
				"plugin %q: version %q is not canonical vMAJOR.MINOR.PATCH",
				name,
				version,
			)
		}
		if rel.Auth == "" {
			rel.Auth = "none"
		}
		if err := validateRelease(name, version, rel); err != nil {
			return err
		}
		def.Versions[version] = rel
	}
	rec, ok := def.Versions[def.RecommendedVersion]
	if !ok || rec.Status != "allowed" {
		return invalidCatalog("plugin %q: recommended_version must be an allowed version", name)
	}
	return nil
}

func validateRelease(name, version string, rel Release) error {
	switch rel.Status {
	case "revoked", "allowed":
	default:
		return invalidCatalog("plugin %q %s: unknown status %q", name, version, rel.Status)
	}
	if rel.Auth != "none" && rel.Auth != "shared" {
		return invalidCatalog("plugin %q %s: unknown auth %q", name, version, rel.Auth)
	}
	if rel.Status == "revoked" {
		return nil
	}
	if len(rel.Platforms) == 0 {
		return invalidCatalog("plugin %q %s: allowed version needs platforms", name, version)
	}
	for platform, asset := range rel.Platforms {
		if !slices.Contains(SupportedPlatforms, platform) {
			return invalidCatalog("plugin %q %s: unsupported platform %q", name, version, platform)
		}
		u, err := url.Parse(asset.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return invalidCatalog(
				"plugin %q %s %s: url must be https without userinfo",
				name,
				version,
				platform,
			)
		}
		if !sha256Pattern.MatchString(asset.ArchiveSHA256) ||
			!sha256Pattern.MatchString(asset.ExecutableSHA256) {
			return invalidCatalog(
				"plugin %q %s %s: digests must be 64 lowercase hex chars",
				name,
				version,
				platform,
			)
		}
		if asset.Executable == "" ||
			strings.ContainsAny(asset.Executable, `/\`) ||
			asset.Executable == "." ||
			asset.Executable == ".." {
			return invalidCatalog(
				"plugin %q %s %s: executable must be a single file name",
				name,
				version,
				platform,
			)
		}
		if strings.HasPrefix(platform, "windows-") != strings.HasSuffix(asset.Executable, ".exe") {
			return invalidCatalog(
				"plugin %q %s %s: .exe suffix must match windows platforms",
				name,
				version,
				platform,
			)
		}
	}
	return nil
}

// Names returns catalog plugin names in sorted order.
func (c *Catalog) Names() []string {
	names := make([]string, 0, len(c.Plugins))
	for name := range c.Plugins {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Resolve finds an allowed release asset. An empty version selects the recommended version.
func (c *Catalog) Resolve(name, version, platform string) (Resolved, error) {
	def, ok := c.Plugins[name]
	if !ok {
		return Resolved{}, UserError(CodeUnknown, fmt.Sprintf("plugin %q is not in the bk-cli catalog", name),
			"Run: bk-cli plugin list")
	}
	if version == "" {
		version = def.RecommendedVersion
	}
	rel, ok := def.Versions[version]
	if !ok || rel.Status != "allowed" {
		hint := "Run: bk-cli plugin list to see approved versions; upgrade bk-cli to get newly approved versions"
		prefixedVersion := "v" + version
		if version != "" && !strings.HasPrefix(version, "v") {
			if _, exists := def.Versions[prefixedVersion]; exists {
				hint = fmt.Sprintf(
					"Use --version %s; plugin versions must include the leading v",
					prefixedVersion,
				)
			}
		}
		return Resolved{}, UserError(
			CodeVersionNotAllowed,
			fmt.Sprintf("plugin %q version %q is not allowed by this bk-cli build", name, version),
			hint,
		)
	}
	asset, ok := rel.Platforms[platform]
	if !ok {
		return Resolved{}, UserError(CodePlatformUnsupported,
			fmt.Sprintf("plugin %q version %s has no approved build for %s", name, version, platform), "")
	}
	return Resolved{
		Name:       name,
		Version:    version,
		Platform:   platform,
		Definition: def,
		Release:    rel,
		Asset:      asset,
	}, nil
}
