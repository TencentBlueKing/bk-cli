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

package credential_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/credential"
)

var _ = Describe("Store", func() {
	var (
		tmpDir string
		key    []byte
	)

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-store-*")
		Expect(err).NotTo(HaveOccurred())
		key = credential.DeriveKeyFrom([]byte("test-key"))
		DeferCleanup(os.RemoveAll, tmpDir)
	})

	Describe("Save/LoadFromFile", func() {
		It("round-trips encrypts and decrypts", func() {
			cred := &credential.Credential{
				Type:        credential.TypeAppUser,
				BkAppCode:   "myapp",
				BkAppSecret: "secret",
				BkToken:     "token",
			}
			path := filepath.Join(tmpDir, "creds.enc")
			Expect(credential.Save(path, cred, key)).To(Succeed())

			loaded, err := credential.LoadFromFile(path, key)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded.Type).To(Equal(cred.Type))
			Expect(loaded.BkAppCode).To(Equal(cred.BkAppCode))
			Expect(loaded.BkAppSecret).To(Equal(cred.BkAppSecret))
			Expect(loaded.BkToken).To(Equal(cred.BkToken))
		})

		It("validates before saving", func() {
			invalid := &credential.Credential{
				Type: credential.TypeAppUser,
				// missing required fields
			}
			path := filepath.Join(tmpDir, "bad.enc")
			err := credential.Save(path, invalid, key)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("bk_app_code"))
		})
	})

	Describe("Delete", func() {
		It("removes file", func() {
			path := filepath.Join(tmpDir, "todelete.enc")
			Expect(os.WriteFile(path, []byte("data"), 0o600)).To(Succeed())
			Expect(credential.Delete(path)).To(Succeed())

			_, err := os.Stat(path)
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("no error if file doesn't exist", func() {
			path := filepath.Join(tmpDir, "nonexistent.enc")
			Expect(credential.Delete(path)).To(Succeed())
		})
	})

	Describe("Exists", func() {
		It("returns true when file exists", func() {
			path := filepath.Join(tmpDir, "exists.enc")
			Expect(os.WriteFile(path, []byte("data"), 0o600)).To(Succeed())
			Expect(credential.Exists(path)).To(BeTrue())
		})

		It("returns false when file does not exist", func() {
			path := filepath.Join(tmpDir, "nope.enc")
			Expect(credential.Exists(path)).To(BeFalse())
		})
	})

	Describe("Error cases", func() {
		It("returns error for corrupted file", func() {
			path := filepath.Join(tmpDir, "corrupted.enc")
			Expect(os.WriteFile(path, []byte("not-valid-encrypted-data"), 0o600)).To(Succeed())

			_, err := credential.LoadFromFile(path, key)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("corrupted"))
		})

		It("returns error for non-existent file", func() {
			path := filepath.Join(tmpDir, "missing.enc")
			_, err := credential.LoadFromFile(path, key)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no credentials found"))
		})
	})
})
