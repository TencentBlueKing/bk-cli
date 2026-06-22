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

package credential

import (
	"fmt"
	"os"
	"path/filepath"
)

// Save encrypts and writes a credential to the given path.
func Save(path string, cred *Credential, key []byte) error {
	if err := cred.Validate(); err != nil {
		return err
	}
	data, err := cred.Marshal()
	if err != nil {
		return err
	}
	encoded, err := Encrypt(data, key)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	return os.WriteFile(path, []byte(encoded), 0o600)
}

// LoadFromFile reads, decrypts, and parses a credential from the given path.
func LoadFromFile(path string, key []byte) (*Credential, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no credentials found at %s. Run: bk-cli auth login", path)
		}
		return nil, fmt.Errorf("failed to read credentials: %w", err)
	}
	plaintext, err := Decrypt(string(data), key)
	if err != nil {
		return nil, fmt.Errorf("credentials file corrupted or unreadable at %s. Run: bk-cli auth login", path)
	}
	cred, err := Unmarshal(plaintext)
	if err != nil {
		return nil, fmt.Errorf("credentials file corrupted at %s. Run: bk-cli auth login", path)
	}
	return cred, nil
}

// Delete removes the credentials file at the given path.
func Delete(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove credentials: %w", err)
	}
	return nil
}

// Exists checks whether a credentials file exists at the given path.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
