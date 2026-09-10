package db

import (
	"path/filepath"
	"testing"
)

func TestRelativePathAndForeignKeys(t *testing.T) {
	t.Chdir(t.TempDir())
	d, err := Open("local #1.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var fk int
	if err := d.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign keys: %d %v", fk, err)
	}
	var name string
	var seq int
	var file string
	if err := d.QueryRow("PRAGMA database_list").Scan(&seq, &name, &file); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(file) != "local #1.db" {
		t.Fatal(file)
	}
}
