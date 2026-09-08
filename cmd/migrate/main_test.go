package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMigrationsSortsAndChecksumsFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "002_second.sql"), []byte("SELECT 2;"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "001_first.sql"), []byte("SELECT 1;"), 0o600); err != nil {
		t.Fatal(err)
	}

	migrations, err := loadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 || migrations[0].version != 1 || migrations[1].version != 2 {
		t.Fatalf("migrations = %+v", migrations)
	}
	if migrations[0].checksum == "" || migrations[0].checksum == migrations[1].checksum {
		t.Fatalf("checksums were not calculated: %+v", migrations)
	}
}

func TestLoadMigrationsRejectsDuplicateVersion(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"001_first.sql", "001_again.sql"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1;"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := loadMigrations(dir); err == nil {
		t.Fatal("expected duplicate migration version error")
	}
}
