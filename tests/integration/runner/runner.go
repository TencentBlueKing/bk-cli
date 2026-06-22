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

// Package runner executes YAML-defined integration scenarios against bk-cli.
package runner

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	json "github.com/goccy/go-json"
	"go.yaml.in/yaml/v3"
)

const (
	CategoryCLIExecution       = "cli_execution"
	CategoryExpectation        = "expectation_mismatch"
	CategoryMockMismatch       = "mock_mismatch"
	CategoryEnvironmentStartup = "environment_startup"
)

type ExitCodeMode string

const (
	ExitCodeExact   ExitCodeMode = "exact"
	ExitCodeNonZero ExitCodeMode = "nonzero"
)

type ExitCodeExpectation struct {
	Mode  ExitCodeMode
	Value int
}

// UnmarshalYAML accepts either an exact integer exit code or the "nonzero" shorthand.
func (e *ExitCodeExpectation) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!int" {
			value, err := strconv.Atoi(node.Value)
			if err != nil {
				return err
			}
			e.Mode = ExitCodeExact
			e.Value = value
			return nil
		}

		switch strings.TrimSpace(node.Value) {
		case "", "0":
			e.Mode = ExitCodeExact
			e.Value = 0
			return nil
		case "nonzero":
			e.Mode = ExitCodeNonZero
			return nil
		default:
			value, err := strconv.Atoi(node.Value)
			if err != nil {
				return fmt.Errorf("unsupported exit_code value %q", node.Value)
			}
			e.Mode = ExitCodeExact
			e.Value = value
			return nil
		}
	default:
		return fmt.Errorf("exit_code must be a scalar value")
	}
}

// Matches reports whether code satisfies the configured exit-code expectation.
func (e ExitCodeExpectation) Matches(code int) bool {
	switch e.Mode {
	case ExitCodeNonZero:
		return code != 0
	default:
		return code == e.Value
	}
}

// Describe returns a human-readable representation of the expected exit code.
func (e ExitCodeExpectation) Describe() string {
	switch e.Mode {
	case ExitCodeNonZero:
		return "non-zero"
	default:
		return strconv.Itoa(e.Value)
	}
}

type CaseFile struct {
	ID        string    `yaml:"id"`
	Name      string    `yaml:"name"`
	Tags      []string  `yaml:"tags"`
	Workspace Workspace `yaml:"workspace"`
	Steps     []Step    `yaml:"steps"`
	Path      string    `yaml:"-"`
}

type Workspace struct {
	SeedConfig string `yaml:"seed_config"`
}

type Step struct {
	Name    string            `yaml:"name"`
	Env     map[string]string `yaml:"env"`
	Command []string          `yaml:"command"`
	Expect  Expectation       `yaml:"expect"`
}

type Expectation struct {
	ExitCode ExitCodeExpectation `yaml:"exit_code"`
	Category string              `yaml:"category"`
	Checks   []Check             `yaml:"checks"`
}

type Check struct {
	Path          string `yaml:"path"`
	Contains      string `yaml:"contains"`
	Kind          string `yaml:"kind"`
	LengthAtLeast *int   `yaml:"length_at_least"`
	Equals        any    `yaml:"equals"`
}

type ExecutionResult struct {
	Command  []string      `json:"command"`
	ExitCode int           `json:"exit_code"`
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	Duration time.Duration `json:"duration"`
}

type ExpectationError struct {
	Category string
	Message  string
	Details  string
}

func (e *ExpectationError) Error() string {
	if e.Details == "" {
		return fmt.Sprintf("[%s] %s", e.Category, e.Message)
	}
	return fmt.Sprintf("[%s] %s :: %s", e.Category, e.Message, e.Details)
}

type RunnerConfig struct {
	BKCLIBin    string
	CasesRoot   string
	ReportDir   string
	FixturesDir string
	Scenario    string
	CasePath    string
}

type Report struct {
	Status  string        `json:"status"`
	Summary ReportSummary `json:"summary"`
	Cases   []CaseReport  `json:"cases"`
}

type ReportSummary struct {
	Total      int   `json:"total"`
	Passed     int   `json:"passed"`
	Failed     int   `json:"failed"`
	DurationMS int64 `json:"duration_ms"`
}

type CaseReport struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Path       string       `json:"path"`
	Status     string       `json:"status"`
	DurationMS int64        `json:"duration_ms"`
	Error      string       `json:"error,omitempty"`
	Category   string       `json:"category,omitempty"`
	Steps      []StepReport `json:"steps"`
}

type StepReport struct {
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	DurationMS int64    `json:"duration_ms"`
	Command    []string `json:"command"`
	ExitCode   int      `json:"exit_code"`
	StdoutFile string   `json:"stdout_file,omitempty"`
	StderrFile string   `json:"stderr_file,omitempty"`
	Error      string   `json:"error,omitempty"`
	Category   string   `json:"category,omitempty"`
}

type junitTestSuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Time     string           `xml:"time,attr"`
	Suites   []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Time      string          `xml:"time,attr"`
	TestCases []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

// LoadCases loads and validates all YAML case files under root.
func LoadCases(root string) ([]CaseFile, error) {
	var cases []CaseFile
	seen := map[string]string{}
	rootFS := os.DirFS(root)

	err := fs.WalkDir(rootFS, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		data, err := fs.ReadFile(rootFS, path)
		if err != nil {
			return err
		}

		var item CaseFile
		if err := yaml.Unmarshal(data, &item); err != nil {
			return fmt.Errorf("parse %s: %w", absolutePath(root, path), err)
		}
		item.Path = absolutePath(root, path)
		if err := validateCase(item); err != nil {
			return fmt.Errorf("%s: %w", item.Path, err)
		}
		if previous, ok := seen[item.ID]; ok {
			return fmt.Errorf("duplicate case id %q in %s and %s", item.ID, previous, item.Path)
		}
		seen[item.ID] = item.Path
		cases = append(cases, item)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(cases, func(i, j int) bool {
		return cases[i].ID < cases[j].ID
	})
	return cases, nil
}

func validateCase(item CaseFile) error {
	if strings.TrimSpace(item.ID) == "" {
		return errors.New("missing case id")
	}
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("missing case name")
	}
	if len(item.Steps) == 0 {
		return errors.New("case must define at least one step")
	}
	for index, step := range item.Steps {
		if strings.TrimSpace(step.Name) == "" {
			return fmt.Errorf("step %d is missing name", index+1)
		}
		if len(step.Command) == 0 {
			return fmt.Errorf("step %q is missing command", step.Name)
		}
	}
	return nil
}

// SelectCases filters cases by scenario ID or case path when requested.
func SelectCases(cases []CaseFile, root, scenario, casePath string) ([]CaseFile, error) {
	if scenario != "" && casePath != "" {
		return nil, errors.New("SCENARIO and CASE are mutually exclusive")
	}
	if scenario != "" {
		for _, item := range cases {
			if item.ID == scenario {
				return []CaseFile{item}, nil
			}
		}
		return nil, fmt.Errorf("scenario %q not found", scenario)
	}
	if casePath != "" {
		candidates := resolveCasePath(root, casePath)
		for _, item := range cases {
			if slices.Contains(candidates, item.Path) {
				return []CaseFile{item}, nil
			}
		}
		return nil, fmt.Errorf("case path %q not found", casePath)
	}
	return cases, nil
}

func resolveCasePath(root, requested string) []string {
	if filepath.IsAbs(requested) {
		return []string{filepath.Clean(requested)}
	}
	candidates := []string{}
	if absolute, err := filepath.Abs(requested); err == nil {
		candidates = append(candidates, filepath.Clean(absolute))
	}
	candidates = append(candidates, filepath.Clean(filepath.Join(root, requested)))
	return candidates
}

// EvaluateExpectations validates one step result against its declarative checks.
func EvaluateExpectations(result ExecutionResult, expect Expectation) error {
	if !expect.ExitCode.Matches(result.ExitCode) {
		return &ExpectationError{
			Category: CategoryCLIExecution,
			Message: fmt.Sprintf(
				"expected exit code %s, got %d",
				expect.ExitCode.Describe(),
				result.ExitCode,
			),
			Details: strings.TrimSpace(firstNonEmpty(result.Stderr, result.Stdout)),
		}
	}

	var envelope any
	if len(expect.Checks) > 0 {
		var ok bool
		envelope, ok = parseEnvelope(result)
		if !ok {
			return &ExpectationError{
				Category: CategoryCLIExecution,
				Message:  "expected JSON envelope in stdout or stderr",
				Details:  strings.TrimSpace(firstNonEmpty(result.Stdout, result.Stderr)),
			}
		}
	}

	category := expect.Category
	if category == "" {
		category = CategoryExpectation
	}

	for _, check := range expect.Checks {
		actual, err := lookupPath(envelope, check.Path)
		if err != nil {
			return &ExpectationError{Category: category, Message: err.Error()}
		}
		if check.Equals != nil && !valuesEqual(actual, check.Equals) {
			return &ExpectationError{
				Category: category,
				Message: fmt.Sprintf(
					"expected %s to equal %s, got %s",
					check.Path,
					stringify(check.Equals),
					stringify(actual),
				),
			}
		}
		if check.Contains != "" {
			value, ok := actual.(string)
			if !ok || !strings.Contains(value, check.Contains) {
				return &ExpectationError{
					Category: category,
					Message: fmt.Sprintf(
						"expected %s to contain %q, got %s",
						check.Path,
						check.Contains,
						stringify(actual),
					),
				}
			}
		}
		if check.Kind != "" {
			if kindOf(actual) != strings.ToLower(check.Kind) {
				return &ExpectationError{
					Category: category,
					Message: fmt.Sprintf(
						"expected %s to be %s, got %s",
						check.Path,
						strings.ToLower(check.Kind),
						kindOf(actual),
					),
				}
			}
		}
		if check.LengthAtLeast != nil {
			length, ok := lengthOf(actual)
			if !ok || length < *check.LengthAtLeast {
				return &ExpectationError{
					Category: category,
					Message: fmt.Sprintf(
						"expected %s length >= %d, got %d",
						check.Path,
						*check.LengthAtLeast,
						length,
					),
				}
			}
		}
	}

	return nil
}

// Run executes the selected integration cases and writes report artifacts.
func Run(config RunnerConfig) (Report, error) {
	started := time.Now()
	if err := os.MkdirAll(config.ReportDir, 0o750); err != nil {
		return Report{}, err
	}

	cases, err := LoadCases(config.CasesRoot)
	if err != nil {
		return Report{}, err
	}
	selected, err := SelectCases(cases, config.CasesRoot, config.Scenario, config.CasePath)
	if err != nil {
		return Report{}, err
	}

	if err := printSelection(selected); err != nil {
		return Report{}, err
	}

	report := Report{Status: "pass"}
	for _, item := range selected {
		caseReport := runCase(config, item)
		report.Cases = append(report.Cases, caseReport)
		if caseReport.Status != "pass" {
			report.Status = "fail"
		}
	}

	report.Summary.Total = len(report.Cases)
	for _, item := range report.Cases {
		if item.Status == "pass" {
			report.Summary.Passed++
		} else {
			report.Summary.Failed++
		}
	}
	report.Summary.DurationMS = time.Since(started).Milliseconds()

	if err := writeJSONReport(filepath.Join(config.ReportDir, "results.json"), report); err != nil {
		return report, err
	}
	if err := writeJUnitReport(filepath.Join(config.ReportDir, "results.xml"), report); err != nil {
		return report, err
	}

	if report.Status != "pass" {
		return report, errors.New("integration cases failed")
	}
	return report, nil
}

func runCase(config RunnerConfig, item CaseFile) CaseReport {
	started := time.Now()
	report := CaseReport{
		ID:     item.ID,
		Name:   item.Name,
		Path:   item.Path,
		Status: "pass",
	}

	runtimeRoot := filepath.Join(config.ReportDir, "runtime", item.ID)
	configDir := filepath.Join(runtimeRoot, "config")
	_ = os.RemoveAll(runtimeRoot)
	if err := os.MkdirAll(runtimeRoot, 0o750); err != nil {
		report.Status = "fail"
		report.Category = CategoryEnvironmentStartup
		report.Error = err.Error()
		return report
	}
	if err := prepareConfigDir(config.FixturesDir, configDir, item.Workspace.SeedConfig); err != nil {
		report.Status = "fail"
		report.Category = CategoryEnvironmentStartup
		report.Error = err.Error()
		return report
	}

	for _, step := range item.Steps {
		stepStarted := time.Now()
		result, stdoutFile, stderrFile, err := executeStep(config, step, configDir, runtimeRoot)
		stepReport := StepReport{
			Name:       step.Name,
			Command:    result.Command,
			ExitCode:   result.ExitCode,
			DurationMS: time.Since(stepStarted).Milliseconds(),
			StdoutFile: stdoutFile,
			StderrFile: stderrFile,
		}
		if err != nil {
			stepReport.Status = "fail"
			stepReport.Category = CategoryCLIExecution
			stepReport.Error = err.Error()
			report.Status = "fail"
			report.Category = CategoryCLIExecution
			report.Error = fmt.Sprintf("%s: %s", step.Name, err.Error())
			report.Steps = append(report.Steps, stepReport)
			break
		}
		if err := EvaluateExpectations(result, step.Expect); err != nil {
			stepReport.Status = "fail"
			stepReport.Error = err.Error()
			var expectationError *ExpectationError
			if errors.As(err, &expectationError) {
				stepReport.Category = expectationError.Category
				report.Category = expectationError.Category
			}
			report.Status = "fail"
			report.Error = fmt.Sprintf("%s: %s", step.Name, err.Error())
			report.Steps = append(report.Steps, stepReport)
			break
		}

		stepReport.Status = "pass"
		report.Steps = append(report.Steps, stepReport)
		fmt.Printf("%s/%s - PASS (%d ms)\n", item.ID, sanitizeName(step.Name), stepReport.DurationMS)
	}

	report.DurationMS = time.Since(started).Milliseconds()
	if report.Status != "pass" {
		fmt.Printf("%s - FAIL: %s\n", item.ID, report.Error)
	}
	return report
}

func executeStep(
	config RunnerConfig,
	step Step,
	configDir, runtimeRoot string,
) (ExecutionResult, string, string, error) {
	command, envMap := expandStep(step, configDir)
	result := ExecutionResult{
		Command: append([]string{config.BKCLIBin}, command...),
	}

	started := time.Now()
	// #nosec G204 -- Integration cases intentionally execute the built bk-cli binary.
	cmd := exec.CommandContext(context.Background(), config.BKCLIBin, command...)
	cmd.Env = envSlice(envMap)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	result.Duration = time.Since(started)
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
		}
	} else {
		result.ExitCode = 0
	}

	baseName := sanitizeName(step.Name)
	stdoutFile := filepath.Join(runtimeRoot, baseName+".stdout.log")
	stderrFile := filepath.Join(runtimeRoot, baseName+".stderr.log")
	_ = os.WriteFile(stdoutFile, []byte(result.Stdout), 0o600)
	_ = os.WriteFile(stderrFile, []byte(result.Stderr), 0o600)

	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			return result, stdoutFile, stderrFile, runErr
		}
	}

	return result, stdoutFile, stderrFile, nil
}

func expandStep(step Step, configDir string) ([]string, map[string]string) {
	envMap := envMapFromSlice(os.Environ())
	for key, value := range step.Env {
		envMap[key] = expandString(value, envMap)
	}
	envMap["BK_CLI_CONFIG_DIR"] = configDir

	command := make([]string, 0, len(step.Command))
	for _, part := range step.Command {
		command = append(command, expandString(part, envMap))
	}

	return command, envMap
}

func prepareConfigDir(fixturesDir, configDir, seed string) error {
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return err
	}
	if seed == "" {
		return nil
	}
	return copyTree(filepath.Join(fixturesDir, "config", seed), configDir)
}

func copyTree(sourceDir, targetDir string) error {
	sourceFS := os.DirFS(sourceDir)
	type entry struct {
		path  string
		isDir bool
	}

	var entries []entry

	if err := fs.WalkDir(sourceFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		entries = append(entries, entry{path: path, isDir: d.IsDir()})
		return nil
	}); err != nil {
		return err
	}

	for _, item := range entries {
		destination := absolutePath(targetDir, item.path)
		if item.isDir {
			if err := os.MkdirAll(destination, 0o750); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
			return err
		}
		data, err := fs.ReadFile(sourceFS, item.path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(destination, data, 0o600); err != nil {
			return err
		}
	}

	return nil
}

func printSelection(cases []CaseFile) error {
	scenarios := make([]string, 0, len(cases))
	for _, item := range cases {
		scenarios = append(scenarios, item.ID)
	}
	payload := map[string]any{
		"count":     len(cases),
		"scenarios": scenarios,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func parseEnvelope(result ExecutionResult) (any, bool) {
	for _, candidate := range []string{result.Stdout, result.Stderr} {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		var value any
		if err := json.Unmarshal([]byte(candidate), &value); err == nil {
			return value, true
		}
	}
	return nil, false
}

func lookupPath(value any, path string) (any, error) {
	if path == "" {
		return value, nil
	}
	current := value
	for part := range strings.SplitSeq(path, ".") {
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[part]
			if !ok {
				return nil, fmt.Errorf("path %s not found", path)
			}
			current = next
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, fmt.Errorf("path %s not found", path)
			}
			current = typed[index]
		default:
			return nil, fmt.Errorf("path %s not found", path)
		}
	}
	return current, nil
}

func valuesEqual(actual, expected any) bool {
	if bothNumeric(actual, expected) {
		return toFloat64(actual) == toFloat64(expected)
	}
	return reflect.DeepEqual(normalizeValue(actual), normalizeValue(expected))
}

func normalizeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, item := range typed {
			normalized[key] = normalizeValue(item)
		}
		return normalized
	case []any:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, normalizeValue(item))
		}
		return items
	default:
		return value
	}
}

func bothNumeric(left, right any) bool {
	return isNumeric(left) && isNumeric(right)
}

func isNumeric(value any) bool {
	switch value.(type) {
	case int, int32, int64, uint, uint32, uint64, float32, float64:
		return true
	default:
		return false
	}
}

func toFloat64(value any) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case uint:
		return float64(typed)
	case uint32:
		return float64(typed)
	case uint64:
		return float64(typed)
	case float32:
		return float64(typed)
	case float64:
		return typed
	default:
		return 0
	}
}

func kindOf(value any) string {
	switch value.(type) {
	case []any:
		return "list"
	case map[string]any:
		return "map"
	case string:
		return "string"
	case bool:
		return "bool"
	case nil:
		return "null"
	default:
		if isNumeric(value) {
			return "number"
		}
		return "unknown"
	}
}

func lengthOf(value any) (int, bool) {
	switch typed := value.(type) {
	case []any:
		return len(typed), true
	case map[string]any:
		return len(typed), true
	case string:
		return len(typed), true
	default:
		return 0, false
	}
}

func stringify(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func envMapFromSlice(values []string) map[string]string {
	result := make(map[string]string, len(values))
	for _, item := range values {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		result[parts[0]] = parts[1]
	}
	return result
}

func envSlice(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]string, 0, len(keys))
	for _, key := range keys {
		items = append(items, key+"="+values[key])
	}
	return items
}

func expandString(value string, env map[string]string) string {
	return os.Expand(value, func(name string) string {
		return env[name]
	})
}

func sanitizeName(value string) string {
	replacer := strings.NewReplacer(" ", "-", "/", "-", "_", "-")
	return strings.ToLower(replacer.Replace(value))
}

func writeJSONReport(path string, report Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func writeJUnitReport(path string, report Report) error {
	suites := junitTestSuites{
		Tests:    len(report.Cases),
		Failures: report.Summary.Failed,
		Time:     formatSeconds(report.Summary.DurationMS),
	}
	for _, item := range report.Cases {
		testCase := junitTestCase{
			Name:      item.ID,
			ClassName: "integration",
			Time:      formatSeconds(item.DurationMS),
		}
		suite := junitTestSuite{
			Name:      item.ID,
			Tests:     1,
			Failures:  0,
			Time:      formatSeconds(item.DurationMS),
			TestCases: []junitTestCase{testCase},
		}
		if item.Status != "pass" {
			suite.Failures = 1
			suite.TestCases[0].Failure = &junitFailure{
				Message: item.Error,
				Type:    item.Category,
				Body:    item.Error,
			}
		}
		suites.Suites = append(suites.Suites, suite)
	}
	data, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(xml.Header), data...), 0o600)
}

func formatSeconds(durationMS int64) string {
	return fmt.Sprintf("%.3f", float64(durationMS)/1000.0)
}

func absolutePath(root, relative string) string {
	return filepath.Clean(filepath.Join(root, filepath.FromSlash(relative)))
}
