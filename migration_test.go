package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationRetriesAndPreservesConflicts(t *testing.T) {
	source, dest := t.TempDir(), t.TempDir()
	old := filepath.Join(source, ".data", "keys.json")
	target := filepath.Join(dest, ".data", "keys.json")
	body, _ := json.Marshal(Vault{Version: 1, Salt: make([]byte, 16), Nonce: make([]byte, 12), Cipher: make([]byte, 16)})
	if err := atomicPrivateWrite(old, body); err != nil {
		t.Fatal(err)
	}
	f := old + ".settings.json"
	if err := atomicPrivateWrite(f, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := migrateWorkbench(source, target); err != nil {
			t.Fatal(err)
		}
	}
	if err := atomicPrivateWrite(f, []byte("{ }")); err != nil {
		t.Fatal(err)
	}
	if err := migrateWorkbench(source, target); err == nil {
		t.Fatal("expected conflict")
	}
	data, err := os.ReadFile(target + ".settings.json")
	if err != nil || string(data) != "{}" {
		t.Fatal("existing settings changed")
	}
}
