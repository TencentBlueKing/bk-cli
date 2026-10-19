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

package output

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"
)

const (
	jqTimeout    = 2 * time.Second
	jqMaxResults = 10000
	jqMaxBytes   = 16 << 20
)

// jqValue compiles during flag parsing, before auth, stdin or remote side effects.
type jqValue struct {
	expression string
	code       *gojq.Code
	halt       error
}

func (v *jqValue) String() string { return v.expression }

// Type names the flag value in help.
func (*jqValue) Type() string { return "expression" }

// Set validates and compiles the expression before command execution.
func (v *jqValue) Set(expression string) error {
	if strings.TrimSpace(expression) == "" {
		return fmt.Errorf("--jq requires a non-empty expression")
	}
	// The native zero-value HaltError represents normal halt (nil value, code 0).
	// Give halt its own error identity so even null | halt_error(0) stays an
	// execution failure, while both operations remain uncatchable by jq.
	query, err := gojq.Parse("def halt: _bk_cli_halt; " + expression)
	if err != nil {
		return fmt.Errorf("invalid --jq expression: %w", err)
	}
	haltErr := &gojq.HaltError{}
	code, err := gojq.Compile(
		query,
		gojq.WithFunction("_bk_cli_halt", 0, 0, func(any, []any) any { return haltErr }),
	)
	if err != nil {
		return fmt.Errorf("invalid --jq expression: %w", err)
	}
	v.expression, v.code, v.halt = expression, code, haltErr
	return nil
}

// AddJQFlag opts an envelope-producing command into local jq projection.
func AddJQFlag(cmd *cobra.Command) {
	cmd.Flags().Var(&jqValue{}, "jq", "Project the final envelope with jq; each result is one JSON line")
}

// WriteCommandEnvelope projects only successful final envelopes. Projection
// failures preserve the completed request's original result; I/O failures stop.
func WriteCommandEnvelope(cmd *cobra.Command, env *Envelope) error {
	flag := cmd.Flags().Lookup("jq")
	if flag == nil || !flag.Changed || !env.OK {
		return env.WriteJSON(cmd.OutOrStdout())
	}
	value, ok := flag.Value.(*jqValue)
	if !ok || value.code == nil {
		return fmt.Errorf("--jq was not compiled")
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	projected, err := value.project(ctx, env)
	if err != nil {
		if writeErr := env.WriteJSON(cmd.OutOrStdout()); writeErr != nil {
			return writeErr
		}
		var cliErr *CLIError
		if !errors.As(err, &cliErr) {
			return err
		}
		message := cliErr.Message
		if !env.DryRun {
			message += "; the request has already completed; this display failure does not mean it was not executed"
		}
		if writeErr := Err(
			cliErr.Code,
			message,
			"Inspect the original result before deciding whether to retry",
		).WriteJSON(
			cmd.ErrOrStderr(),
		); writeErr != nil {
			return writeErr
		}
		return &CLIError{ExitCode: ExitCodeSystemError, Code: cliErr.Code, Message: message}
	}
	if len(projected) == 0 {
		return nil
	}
	n, err := cmd.OutOrStdout().Write(projected)
	if err == nil && n != len(projected) {
		return io.ErrShortWrite
	}
	return err
}

func (v *jqValue) project(parent context.Context, env *Envelope) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, jqTimeout)
	defer cancel()
	data, err := json.Marshal(env)
	if err != nil {
		return nil, jqError("jq_execution_error", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var input any
	if err = decoder.Decode(&input); err != nil {
		return nil, jqError("jq_execution_error", err)
	}
	var buffer bytes.Buffer
	iter := v.code.RunWithContext(ctx, input)
	for count := 0; ; count++ {
		if err = ctx.Err(); err != nil {
			return nil, jqContextError(parent, err)
		}
		result, more := iter.Next()
		if !more {
			break
		}
		if resultErr, ok := result.(error); ok {
			if errors.Is(resultErr, v.halt) {
				break
			}
			if ctx.Err() != nil {
				return nil, jqContextError(parent, ctx.Err())
			}
			return nil, jqError("jq_execution_error", resultErr)
		}
		if count >= jqMaxResults {
			return nil, jqError("jq_limit_exceeded", fmt.Errorf("more than %d results", jqMaxResults))
		}
		encoded, encodeErr := gojq.Marshal(result)
		if encodeErr != nil {
			return nil, jqError("jq_execution_error", encodeErr)
		}
		if len(encoded)+1 > jqMaxBytes-buffer.Len() {
			return nil, jqError("jq_limit_exceeded", fmt.Errorf("output exceeds %d bytes", jqMaxBytes))
		}
		buffer.Write(encoded)
		buffer.WriteByte('\n')
	}
	if err = ctx.Err(); err != nil {
		return nil, jqContextError(parent, err)
	}
	return buffer.Bytes(), nil
}

func jqContextError(parent context.Context, err error) error {
	if parent.Err() != nil {
		return jqError("jq_execution_error", parent.Err())
	}
	return jqError("jq_limit_exceeded", err)
}

func jqError(code string, err error) error {
	return &CLIError{ExitCode: ExitCodeSystemError, Code: code, Message: err.Error()}
}
