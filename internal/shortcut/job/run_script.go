// Package job contains reusable Job shortcut workflows shared by CLI surfaces.
package job

import (
	"fmt"

	cmdbsvc "github.com/TencentBlueKing/bk-cli/internal/cmdb"
	jobsvc "github.com/TencentBlueKing/bk-cli/internal/job"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
)

type RunScriptInput struct {
	BizID          int
	Hosts          string
	ScriptContent  string
	ScriptFile     string
	ScriptLanguage string
	AccountAlias   string
	ScriptParam    string
	Timeout        int
	TaskName       string
	Stage          string
	Headers        []string
}

// RunScript resolves business hosts from CMDB, then dispatches a Job fast_execute_script request.
func RunScript(runtime *syslib.Runtime, input RunScriptInput) (*syslib.RequestResult, error) {
	if runtime == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	if runtime.DryRun {
		return buildDryRun(runtime, input)
	}

	resolved, err := cmdbsvc.ResolveBizHosts(runtime, cmdbsvc.ResolveBizHostsInput{
		BizID:   input.BizID,
		Hosts:   input.Hosts,
		Stage:   input.Stage,
		Headers: input.Headers,
	})
	if err != nil {
		return nil, err
	}

	hostIDs := make([]int64, 0, len(resolved.HostIDs))
	hostIDs = append(hostIDs, resolved.HostIDs...)
	return jobsvc.FastExecuteScript(runtime, jobsvc.FastExecuteScriptInput{
		BizID:          input.BizID,
		ScriptContent:  input.ScriptContent,
		ScriptFile:     input.ScriptFile,
		ScriptLanguage: input.ScriptLanguage,
		TargetServer:   map[string]any{"host_id_list": hostIDs},
		AccountAlias:   input.AccountAlias,
		ScriptParam:    input.ScriptParam,
		Timeout:        input.Timeout,
		TaskName:       input.TaskName,
		Stage:          input.Stage,
		Headers:        input.Headers,
	})
}

func buildDryRun(runtime *syslib.Runtime, input RunScriptInput) (*syslib.RequestResult, error) {
	cmdbSpec, _, err := cmdbsvc.BuildBizHostsRequest(cmdbsvc.ResolveBizHostsInput{
		BizID:   input.BizID,
		Hosts:   input.Hosts,
		Stage:   input.Stage,
		Headers: input.Headers,
	})
	if err != nil {
		return nil, err
	}

	jobSpec, err := jobsvc.BuildFastExecuteScriptRequest(jobsvc.FastExecuteScriptInput{
		BizID:          input.BizID,
		ScriptContent:  input.ScriptContent,
		ScriptFile:     input.ScriptFile,
		ScriptLanguage: input.ScriptLanguage,
		TargetServer: map[string]any{
			"host_id_list": "<derived from resolve_hosts step>",
		},
		AccountAlias: input.AccountAlias,
		ScriptParam:  input.ScriptParam,
		Timeout:      input.Timeout,
		TaskName:     input.TaskName,
		Stage:        input.Stage,
		Headers:      input.Headers,
	})
	if err != nil {
		return nil, err
	}

	cmdbPreview, err := syslib.ExecuteRequest(runtime, cmdbSpec)
	if err != nil {
		return nil, err
	}
	jobPreview, err := syslib.ExecuteRequest(runtime, jobSpec)
	if err != nil {
		return nil, err
	}

	env := output.SuccessData(map[string]any{
		"shortcut": "job.+run-script",
		"steps": []map[string]any{
			{"name": "resolve_hosts", "request": cmdbPreview.DryRunRequest},
			{"name": "fast_execute_script", "request": jobPreview.DryRunRequest},
		},
	})
	env.DryRun = true

	return &syslib.RequestResult{Envelope: env}, nil
}
