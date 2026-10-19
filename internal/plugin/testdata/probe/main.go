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

// Test-only child process. It only ever sees synthetic credentials.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "exit":
			code, _ := strconv.Atoi(os.Args[2])
			fmt.Fprintln(os.Stderr, "probe-exit")
			os.Exit(code)
		case "sleep":
			fmt.Println("ready")
			time.Sleep(30 * time.Second)
			return
		case "kill-self":
			p, _ := os.FindProcess(os.Getpid())
			_ = p.Signal(syscall.SIGTERM)
			time.Sleep(5 * time.Second)
			return
		}
	}
	input, _ := io.ReadAll(os.Stdin)
	cwd, _ := os.Getwd()
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"args":     os.Args[1:],
		"stdin":    string(input),
		"cwd":      cwd,
		"protocol": os.Getenv("BK_CLI_PLUGIN_PROTOCOL"),
		"context":  os.Getenv("BK_CLI_PLUGIN_CONTEXT"),
		"auth":     os.Getenv("BK_CLI_PLUGIN_AUTH"),
	})
}
