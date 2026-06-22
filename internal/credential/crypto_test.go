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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/credential"
)

var _ = Describe("Crypto", func() {
	Describe("Encrypt/Decrypt", func() {
		var key []byte

		BeforeEach(func() {
			key = credential.DeriveKeyFrom([]byte("test-passphrase"))
		})

		It("round-trips with valid key", func() {
			plaintext := []byte("hello, world!")
			encrypted, err := credential.Encrypt(plaintext, key)
			Expect(err).NotTo(HaveOccurred())
			Expect(encrypted).NotTo(BeEmpty())

			decrypted, err := credential.Decrypt(encrypted, key)
			Expect(err).NotTo(HaveOccurred())
			Expect(decrypted).To(Equal(plaintext))
		})

		It("fails to decrypt with wrong key", func() {
			plaintext := []byte("secret data")
			encrypted, err := credential.Encrypt(plaintext, key)
			Expect(err).NotTo(HaveOccurred())

			wrongKey := credential.DeriveKeyFrom([]byte("wrong-passphrase"))
			_, err = credential.Decrypt(encrypted, wrongKey)
			Expect(err).To(HaveOccurred())
		})

		It("fails to decrypt corrupted data", func() {
			plaintext := []byte("secret data")
			encrypted, err := credential.Encrypt(plaintext, key)
			Expect(err).NotTo(HaveOccurred())

			// Corrupt the base64 content by changing characters
			corrupted := "AAAA" + encrypted[4:]
			_, err = credential.Decrypt(corrupted, key)
			Expect(err).To(HaveOccurred())
		})

		It("produces different ciphertexts for same plaintext (nonce uniqueness)", func() {
			plaintext := []byte("same input")
			enc1, err := credential.Encrypt(plaintext, key)
			Expect(err).NotTo(HaveOccurred())
			enc2, err := credential.Encrypt(plaintext, key)
			Expect(err).NotTo(HaveOccurred())

			Expect(enc1).NotTo(Equal(enc2))
		})
	})

	Describe("DeriveKeyFrom", func() {
		It("produces a 32-byte key", func() {
			key := credential.DeriveKeyFrom([]byte("seed"))
			Expect(key).To(HaveLen(32))
		})

		It("same input produces same key", func() {
			key1 := credential.DeriveKeyFrom([]byte("deterministic"))
			key2 := credential.DeriveKeyFrom([]byte("deterministic"))
			Expect(key1).To(Equal(key2))
		})

		It("different inputs produce different keys", func() {
			key1 := credential.DeriveKeyFrom([]byte("input-a"))
			key2 := credential.DeriveKeyFrom([]byte("input-b"))
			Expect(key1).NotTo(Equal(key2))
		})
	})
})
