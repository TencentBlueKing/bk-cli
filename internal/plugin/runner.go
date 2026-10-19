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
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
)

// Streams are the stdio handles given to the plugin process.
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// ExitStatus reports that the plugin process ran and exited; its output was already written.
type ExitStatus struct{ Code int }

func (e *ExitStatus) Error() string { return fmt.Sprintf("plugin exited with status %d", e.Code) }

// Run verifies the installed plugin, then resolves context and credentials, then runs it.
// The order matters: nothing sensitive is read unless the executable matches the catalog.
func (m *Manager) Run(name, contextOverride string, args []string, streams Streams) error {
	r, binary, err := m.VerifiedExecutable(name)
	if err != nil {
		return err
	}
	session, err := LoadSession(contextOverride, r.Release.Auth)
	if err != nil {
		return err
	}
	env, err := BuildEnv(os.Environ(), session)
	if err != nil {
		return err
	}
	// Ignoring host signals would affect the child across exec; Notify keeps the child's defaults.
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, hostSignals()...)
	defer signal.Stop(signals)
	return runProcess(binary, args, env, streams, signals)
}

func runProcess(binary string, args, env []string, streams Streams, signals <-chan os.Signal) error {
	// #nosec G204 -- binary is the digest-verified managed plugin executable.
	cmd := exec.CommandContext(context.Background(), binary, args...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = streams.In, streams.Out, streams.Err
	if err := cmd.Start(); err != nil {
		return SystemError(CodeLaunchFailed, fmt.Sprintf("cannot start plugin: %v", err), "")
	}
	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-signals:
				forwardSignal(cmd.Process, sig)
			case <-done:
				return
			}
		}
	}()
	err := cmd.Wait()
	close(done)
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &ExitStatus{Code: exitCodeFromState(exitErr.ProcessState)}
	}
	return SystemError(CodeLaunchFailed, fmt.Sprintf("plugin process failed: %v", err), "")
}
