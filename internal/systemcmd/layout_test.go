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

package systemcmd

import (
	"os"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("systemcmd package layout", func() {
	It("keeps shared helpers split by concern", func() {
		_, thisFile, _, ok := runtime.Caller(0)
		Expect(ok).To(BeTrue())

		dir := filepath.Dir(thisFile)

		for _, name := range []string{
			"validator.go",
			"parser.go",
			"executor.go",
			"runtime.go",
			"flag.go",
			"payload.go",
		} {
			_, err := os.Stat(filepath.Join(dir, name))
			Expect(err).NotTo(HaveOccurred(), "expected %s to exist", name)
		}

		_, err := os.Stat(filepath.Join(dir, "helpers.go"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "expected helpers.go to be removed")
	})
})
