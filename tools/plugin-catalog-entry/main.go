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

// Command plugin-catalog-entry prints a catalog.yaml version entry for an approved plugin release.
//
// Example:
//
//	go run ./tools/plugin-catalog-entry \
//	  --release-base-url https://example.com/releases/download/plugin/v1.0.4 \
//	  --asset-template 'plugin_{version}_{os}_{arch}' \
//	  --executable plugin --version v1.0.4 --auth none
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/TencentBlueKing/bk-cli/internal/plugin"
)

func main() {
	os.Exit(run())
}

func run() int {
	var opts plugin.EntryOptions
	flag.StringVar(
		&opts.ReleaseBaseURL,
		"release-base-url",
		"",
		"release download base URL containing checksums.txt",
	)
	flag.StringVar(
		&opts.AssetTemplate,
		"asset-template",
		"",
		"archive name without extension, e.g. bkms-cli_{version}_{os}_{arch}",
	)
	flag.StringVar(&opts.Executable, "executable", "", "executable name inside the archive, without .exe")
	flag.StringVar(&opts.Version, "version", "", "exact version, e.g. v1.0.4")
	flag.StringVar(
		&opts.Auth,
		"auth",
		"none",
		"none or shared; shared requires the protocol review in docs/plugin-protocol.md",
	)
	flag.Parse()
	if opts.ReleaseBaseURL == "" || opts.AssetTemplate == "" || opts.Executable == "" || opts.Version == "" {
		flag.Usage()
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	rel, err := plugin.BuildReleaseEntry(ctx, nil, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	out, err := yaml.Marshal(map[string]plugin.Release{opts.Version: rel})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Print(string(out))
	return 0
}
