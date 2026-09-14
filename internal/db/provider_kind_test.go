package db

import (
	"path/filepath"
	"testing"
)

func TestConnectionProviderKindMigrationIsIdempotentAndPreservesRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Conn().Exec(`INSERT INTO connections(id,name,base_url,api_key) VALUES('existing','Existing','https://example.test','secret')`); err != nil {
		t.Fatal(err)
	}
	first.Close()

	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var name, key, kind string
	if err := second.Conn().QueryRow(`SELECT name,api_key,provider_kind FROM connections WHERE id='existing'`).Scan(&name, &key, &kind); err != nil {
		t.Fatal(err)
	}
	if name != "Existing" || key != "secret" || kind != "llm" {
		t.Fatalf("row changed across migration: name=%q key=%q kind=%q", name, key, kind)
	}
}
