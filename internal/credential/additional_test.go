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

var _ = Describe("additional credential coverage", func() {
	It("derives a machine key", func() {
		key, err := credential.DeriveKey()
		Expect(err).NotTo(HaveOccurred())
		Expect(key).To(HaveLen(32))
	})

	It("rejects invalid credential JSON during unmarshal", func() {
		_, err := credential.Unmarshal([]byte(`{bad`))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to parse credential"))
	})

	It("rejects invalid encryption keys", func() {
		_, err := credential.Encrypt([]byte("demo"), []byte("short"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create cipher"))
	})

	It("rejects ciphertexts that are too short", func() {
		_, err := credential.Decrypt("YWJj", credential.DeriveKeyFrom([]byte("seed")))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ciphertext too short"))
	})

	It("rejects decryption with an invalid AES key length", func() {
		encrypted, err := credential.Encrypt([]byte("demo"), credential.DeriveKeyFrom([]byte("seed")))
		Expect(err).NotTo(HaveOccurred())

		_, err = credential.Decrypt(encrypted, []byte("short"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create cipher"))
	})

	It("rejects invalid base64 ciphertext", func() {
		_, err := credential.Decrypt("not-base64", credential.DeriveKeyFrom([]byte("seed")))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to decode base64"))
	})

	It("rejects decrypted payloads that are not valid credential JSON", func() {
		tmpDir := GinkgoT().TempDir()
		path := filepath.Join(tmpDir, "bad.enc")
		key := credential.DeriveKeyFrom([]byte("seed"))
		encoded, err := credential.Encrypt([]byte("not-json"), key)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(path, []byte(encoded), 0o600)).To(Succeed())

		_, err = credential.LoadFromFile(path, key)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("credentials file corrupted"))
	})

	It("returns an error when deleting a non-empty directory", func() {
		tmpDir := GinkgoT().TempDir()
		dir := filepath.Join(tmpDir, "nested")
		Expect(os.MkdirAll(dir, 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o600)).To(Succeed())

		err := credential.Delete(dir)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to remove credentials"))
	})

	It("returns an error when saving with an invalid encryption key", func() {
		tmpDir := GinkgoT().TempDir()
		err := credential.Save(filepath.Join(tmpDir, "creds.enc"), &credential.Credential{
			Type:        credential.TypeAccessToken,
			AccessToken: "token-123",
		}, []byte("short"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create cipher"))
	})

	It("returns an error when the credentials path parent cannot be created", func() {
		tmpDir := GinkgoT().TempDir()
		blocker := filepath.Join(tmpDir, "blocked")
		Expect(os.WriteFile(blocker, []byte("file"), 0o600)).To(Succeed())

		err := credential.Save(filepath.Join(blocker, "creds.enc"), &credential.Credential{
			Type:        credential.TypeAccessToken,
			AccessToken: "token-123",
		}, credential.DeriveKeyFrom([]byte("seed")))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create directory"))
	})

	It("returns an error when the credential path points to a directory", func() {
		tmpDir := GinkgoT().TempDir()
		dir := filepath.Join(tmpDir, "creds")
		Expect(os.MkdirAll(dir, 0o700)).To(Succeed())

		_, err := credential.LoadFromFile(dir, credential.DeriveKeyFrom([]byte("seed")))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to read credentials"))
	})
})
