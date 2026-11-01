/*
Copyright 2026 Joseph Anthony Abbott III

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pkg

import (
	"errors"
	"fmt"
)

// The key-store flows shared by the CLI and the TUI (plan 3.3): generating a stored
// key, unlocking one to encrypt or decrypt with it (plan 3.4), and importing an export.
// They take the key store as a Storage and do the checks that protect data, so both
// interfaces get them; prompting and output-path checks stay in the interfaces.
// Exporting is ExportKeyToFile (crypto.go), after the interface has looked the key up
// and checked the output path.

var (
	// ErrWrongMasterPassword is returned when a stored key doesn't open with the password
	// given: the wrong master password, or a damaged key.
	ErrWrongMasterPassword = errors.New("decryption failed: wrong master password or corrupted key")

	// ErrWrongExportPassword is returned when a key export doesn't open with the password
	// given: the wrong password, or a damaged file.
	ErrWrongExportPassword = errors.New("decryption failed: wrong password for this key export, or the file is corrupted")

	// ErrSeparateKeyPassword is returned by ImportStoredKey when an export opened with
	// the password given but the key inside it didn't (BUG-011): versions up to v1.3.1
	// let an export have a password of its own. The caller can ask for the other
	// password and call ImportStoredKey again with both.
	ErrSeparateKeyPassword = errors.New("the key in this export has a different master password from the export")
)

// GenerateStoredKey generates a random key, protects it with masterPassword, which must
// meet the password policy (CheckPasswordPolicy), and saves it in store.
func GenerateStoredKey(store Storage, masterPassword string) (*KeyModel, error) {
	rawKey, err := GenerateKey()
	if err != nil {
		return nil, err
	}
	blob, err := EncryptKeyBlob(rawKey, masterPassword)
	if err != nil {
		return nil, err
	}
	keyID, err := newKeyID()
	if err != nil {
		return nil, fmt.Errorf("generate key ID: %w", err)
	}
	km := &KeyModel{
		KeyID:         keyID,
		Algorithm:     keyAlgorithm,
		EncryptedBlob: blob,
		CreatedAt_:    timeNow().Unix(),
	}
	if err := store.SaveKey(km); err != nil {
		return nil, fmt.Errorf("save key: %w", err)
	}
	return km, nil
}

// StoredKeyCredential looks up keyID in store and unlocks it with masterPassword,
// returning the credential that encrypts or decrypts files with that key (plan 3.4).
// To encrypt (forNewData), the master password must also meet the password policy, as
// any password that protects new data must (SEC-001): a key stored by an earlier version
// under a shorter password still decrypts, but doesn't encrypt anything new. Only a
// credential unlocked for new data is accepted by EncryptFileWithCredentialContext.
func StoredKeyCredential(store Storage, keyID, masterPassword string, forNewData bool) (Credential, error) {
	km, err := store.GetKey(keyID)
	if err != nil {
		return Credential{}, keyLookupError(keyID, err)
	}
	key, err := DecryptKeyBlob(km.EncryptedBlob, masterPassword)
	if err != nil {
		return Credential{}, fmt.Errorf("unlock key %s: %w", displayText(keyID), err)
	}
	if forNewData {
		if err := CheckPasswordPolicy(masterPassword); err != nil {
			return Credential{}, fmt.Errorf("key %s can't encrypt new files, because its master password doesn't meet the password policy (generate a new key): %w", displayText(keyID), err)
		}
	}
	return newStoredKeyCredential(km.KeyID, key, forNewData)
}

// ImportStoredKey reads the key export at path, checks that the key inside it opens,
// and saves it in store (BUG-011). password is the key's master password, which opens
// the exports this version writes (Q-015). An export written by v1.3.1 or earlier can
// have a password of its own: give it in otherPasswords. Every password given is tried
// on the export and on the key inside, so their order doesn't matter. When the export
// opens but the key inside doesn't, and no other password was given, the error wraps
// ErrSeparateKeyPassword; when no password opens the export, ErrWrongExportPassword.
// The password policy isn't checked, because importing protects nothing new.
func ImportStoredKey(store Storage, path, password string, otherPasswords ...string) (*KeyModel, error) {
	candidates := append([]string{password}, otherPasswords...)
	var km *KeyModel
	var err error
	for _, p := range candidates {
		if km, err = ImportKeyFromFile(path, p); !errors.Is(err, ErrWrongExportPassword) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	for _, p := range candidates {
		if _, err = DecryptKeyBlob(km.EncryptedBlob, p); !errors.Is(err, ErrWrongMasterPassword) {
			break
		}
	}
	switch {
	case errors.Is(err, ErrWrongMasterPassword) && len(otherPasswords) == 0:
		return nil, fmt.Errorf("import key %s: %w", km.KeyID, ErrSeparateKeyPassword)
	case err != nil:
		return nil, fmt.Errorf("import key %s: %w", km.KeyID, err)
	}
	if err := store.SaveKey(km); err != nil {
		return nil, keySaveError(km.KeyID, err)
	}
	return km, nil
}

// keyLookupError explains a failed key lookup, naming the key (SEC-017). The ID is
// quoted, so control characters in it are escaped.
func keyLookupError(keyID string, err error) error {
	if errors.Is(err, ErrKeyNotFound) {
		return fmt.Errorf("%w: %q", ErrKeyNotFound, keyID)
	}
	return fmt.Errorf("look up key %q: %w", keyID, err)
}

// keySaveError explains a failed save of an imported key, naming the key (SEC-017).
func keySaveError(keyID string, err error) error {
	if errors.Is(err, ErrKeyExists) {
		return fmt.Errorf("%w: %q", ErrKeyExists, keyID)
	}
	return fmt.Errorf("save imported key %q: %w", keyID, err)
}
