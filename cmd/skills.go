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
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

	"github.com/TencentBlueKing/bk-cli/internal/output"
)

var (
	embeddedSkillsFS fs.FS
	skillNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
)

// SetSkillsFS sets the embedded skill content filesystem used by `bk-cli skills`.
func SetSkillsFS(fsys fs.FS) {
	embeddedSkillsFS = fsys
}

type skillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path"`
}

type skillFrontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Read embedded bk-cli skill content (list / read)",
		Long: `Read agent-readable skill content embedded in the bk-cli binary.

The embedded content is versioned with this CLI build, so agents can inspect the
matching command usage guidance without downloading files separately.

Examples:
  bk-cli skills list
  bk-cli skills read bk-cli-shared
  bk-cli skills read bk-cli-shared/references/api-debug.md`,
	}
	cmd.AddCommand(newSkillsListCmd())
	cmd.AddCommand(newSkillsReadCmd())
	return cmd
}

func newSkillsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List embedded bk-cli skills",
		Long: `List embedded bk-cli skills.

Examples:
  bk-cli skills list`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			skills, err := listEmbeddedSkills()
			if err != nil {
				return err
			}
			data := map[string]any{
				"skills": skills,
				"count":  len(skills),
			}
			return output.SuccessData(data).WriteJSON(cmd.OutOrStdout())
		},
	}
	cmd.Flags().Bool("json", false, "No-op; list output is always JSON")
	return cmd
}

func newSkillsReadCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "read <name>[/<path>] [path]",
		Short: "Print an embedded skill file",
		Long: `Print a skill's SKILL.md, or a reference file under that skill.

By default this command writes raw file content to stdout so agents can consume
the markdown directly. Use --json when a structured envelope is required.

Examples:
  bk-cli skills read bk-cli-shared
  bk-cli skills read bk-cli-shared references/api-debug.md
  bk-cli skills read bk-cli-shared/references/api-debug.md
  bk-cli skills read bk-cli-shared --json`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, relPath, err := parseSkillReadTarget(args)
			if err != nil {
				return err
			}
			content, listedPath, err := readEmbeddedSkillFile(name, relPath)
			if err != nil {
				return err
			}
			if asJSON {
				data := map[string]any{
					"skill":   name,
					"path":    listedPath,
					"content": string(content),
				}
				return output.SuccessData(data).WriteJSON(cmd.OutOrStdout())
			}
			_, err = cmd.OutOrStdout().Write(content)
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output a JSON envelope instead of raw markdown")
	return cmd
}

func listEmbeddedSkills() ([]skillInfo, error) {
	if embeddedSkillsFS == nil {
		return nil, output.SystemError(
			"skills_not_embedded",
			"skill content is not embedded in this build",
			"Use a release build of bk-cli, or rebuild with embedded skills",
		)
	}

	entries, err := fs.ReadDir(embeddedSkillsFS, "skills")
	if err != nil {
		return nil, output.SystemError("skills_read_failed", err.Error(), "")
	}

	skills := make([]skillInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		content, err := fs.ReadFile(embeddedSkillsFS, path.Join("skills", name, "SKILL.md"))
		if err != nil {
			if errorsIsNotExist(err) {
				continue
			}
			return nil, output.SystemError("skills_read_failed", err.Error(), "")
		}
		info := skillInfo{Name: name, Path: path.Join("skills", name, "SKILL.md")}
		if meta, ok := parseSkillFrontMatter(content); ok {
			if meta.Name != "" {
				info.Name = meta.Name
			}
			info.Description = meta.Description
		}
		skills = append(skills, info)
	}

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})
	return skills, nil
}

func parseSkillReadTarget(args []string) (string, string, error) {
	switch len(args) {
	case 1:
		name, relPath := splitSkillArg(args[0])
		return name, relPath, validateSkillReadPath(name, relPath)
	case 2:
		return args[0], args[1], validateSkillReadPath(args[0], args[1])
	default:
		return "", "", output.UserError(
			"invalid_argument",
			"read requires 1 or 2 arguments: <name>[/<path>] [path]",
			"Run: bk-cli skills read --help",
		)
	}
}

func splitSkillArg(arg string) (string, string) {
	name, relPath, found := strings.Cut(arg, "/")
	if !found {
		return name, ""
	}
	return name, relPath
}

func validateSkillReadPath(name, relPath string) error {
	if !skillNamePattern.MatchString(name) {
		return output.UserError(
			"invalid_skill_path",
			fmt.Sprintf("invalid skill path: %q", name),
			"Use a skill name from `bk-cli skills list`",
		)
	}

	if relPath == "" {
		return nil
	}
	cleaned := path.Clean(relPath)
	if cleaned == "." ||
		path.IsAbs(relPath) ||
		strings.HasPrefix(cleaned, "../") ||
		cleaned == ".." ||
		strings.HasPrefix(relPath, "../") ||
		strings.Contains(relPath, "/../") {
		return output.UserError(
			"invalid_skill_path",
			fmt.Sprintf("invalid skill path: %q", relPath),
			"Use a file path inside the selected skill directory",
		)
	}
	return nil
}

func readEmbeddedSkillFile(name, relPath string) ([]byte, string, error) {
	if embeddedSkillsFS == nil {
		return nil, "", output.SystemError(
			"skills_not_embedded",
			"skill content is not embedded in this build",
			"Use a release build of bk-cli, or rebuild with embedded skills",
		)
	}

	listedPath := "SKILL.md"
	if relPath != "" {
		listedPath = path.Clean(relPath)
	}
	content, err := fs.ReadFile(embeddedSkillsFS, path.Join("skills", name, listedPath))
	if err != nil {
		if errorsIsNotExist(err) {
			return nil, "", output.UserError(
				"skill_not_found",
				fmt.Sprintf("skill file not found: %s/%s", name, listedPath),
				"Run `bk-cli skills list` to see available skills",
			)
		}
		return nil, "", output.SystemError("skills_read_failed", err.Error(), "")
	}
	return content, listedPath, nil
}

func parseSkillFrontMatter(content []byte) (skillFrontMatter, bool) {
	text := string(content)
	if !strings.HasPrefix(text, "---\n") {
		return skillFrontMatter{}, false
	}
	end := strings.Index(text[len("---\n"):], "\n---")
	if end < 0 {
		return skillFrontMatter{}, false
	}

	var meta skillFrontMatter
	if err := yaml.Unmarshal([]byte(text[len("---\n"):len("---\n")+end]), &meta); err != nil {
		return skillFrontMatter{}, false
	}
	return meta, true
}

func errorsIsNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}
