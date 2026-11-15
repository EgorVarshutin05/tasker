package repository_test

import (
	"context"
	"errors"
	"example.com/tasks/internal/model"
	"example.com/tasks/internal/repository"
	"example.com/tasks/internal/service"
	"example.com/tasks/internal/testutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestI01RepositoryCreateList(t *testing.T) {
	db, path := testutil.Fixture(t, false)
	r := repository.New(db)
	ctx := context.Background()
	independent := testutil.Independent(t, path)
	if n := testutil.Scalar(t, independent, "SELECT COUNT(*) FROM sqlite_sequence WHERE name='tasks'"); n != 0 {
		t.Fatalf("sequence rows=%d", n)
	}
	rows, err := r.List(ctx)
	if err != nil || rows == nil || len(rows) != 0 {
		t.Fatalf("empty list=%#v,%v", rows, err)
	}
	want := model.Task{ID: 1, Title: "Отчет"}
	got, err := r.Create(ctx, "Отчет")
	if err != nil || got != want {
		t.Fatalf("create=%#v,%v", got, err)
	}
	rows, err = r.List(ctx)
	if err != nil || !reflect.DeepEqual(rows, []model.Task{want}) {
		t.Fatalf("list=%#v,%v", rows, err)
	}
	testutil.Rows(t, independent, []model.Task{want})
	if n := testutil.Scalar(t, independent, "SELECT COUNT(*) FROM tasks"); n != 1 {
		t.Fatalf("count=%d", n)
	}
	if db.Stats().MaxOpenConnections != 1 {
		t.Fatal("connection limit")
	}
	if n := testutil.Scalar(t, db, "PRAGMA busy_timeout"); n != 5000 {
		t.Fatalf("timeout=%d", n)
	}
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal=%q,%v", mode, err)
	}
}
func TestI02ServiceCompleteDelete(t *testing.T) {
	db, path := testutil.Fixture(t, true)
	check := testutil.Independent(t, path)
	svc := service.New(repository.New(db))
	ctx := context.Background()
	want := model.Task{ID: 1, Title: "Отчет", Done: true}
	for i := 0; i < 2; i++ {
		got, err := svc.Complete(ctx, 1)
		if got != want || err != nil {
			t.Fatalf("complete=%#v,%v", got, err)
		}
		testutil.Rows(t, check, []model.Task{want})
	}
	if err := svc.Delete(ctx, 1); err != nil {
		t.Fatal(err)
	}
	testutil.Rows(t, check, []model.Task{})
	if err := svc.Delete(ctx, 1); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("delete=%v", err)
	}
	testutil.Rows(t, check, []model.Task{})
}
func TestI07SchemaAndStorageFailures(t *testing.T) {
	t.Run("checks", func(t *testing.T) {
		db, _ := testutil.Fixture(t, false)
		for _, q := range []string{
			"INSERT INTO tasks(id,title,done) VALUES(0,'A',0)",
			"INSERT INTO tasks(id,title,done) VALUES(2147483648,'A',0)",
			"INSERT INTO tasks(title,done) VALUES('',0)",
			"INSERT INTO tasks(title,done) VALUES('A',2)",
			"INSERT INTO tasks(title,done) VALUES(NULL,0)",
		} {
			if _, err := db.Exec(q); err == nil {
				t.Fatalf("CHECK accepted %s", q)
			}
		}
		if _, err := db.Exec("INSERT INTO tasks(title,done) VALUES(?,0)", string(make([]byte, 101))); err == nil {
			t.Fatal("NUL-length check must reject empty SQL length")
		}
	})
	t.Run("autoincrement", func(t *testing.T) {
		db, _ := testutil.Fixture(t, true)
		r := repository.New(db)
		ctx := context.Background()
		if err := r.Delete(ctx, 1); err != nil {
			t.Fatal(err)
		}
		got, err := r.Create(ctx, "Новая")
		if err != nil || got.ID != 2 {
			t.Fatalf("id=%d err=%v", got.ID, err)
		}
	})
	t.Run("missing_complete", func(t *testing.T) {
		db, _ := testutil.Fixture(t, false)
		got, err := repository.New(db).Complete(context.Background(), 9)
		if got != (model.Task{}) || !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("got=%#v,%v", got, err)
		}
	})
	t.Run("open_failure", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "file"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		db, err := repository.Open(filepath.Join(root, "file", "tasks.db"))
		if db != nil || !errors.Is(err, model.ErrStorage) {
			t.Fatalf("open=%v,%v", db, err)
		}
	})
	t.Run("drop_table", func(t *testing.T) {
		db, _ := testutil.Fixture(t, false)
		r := repository.New(db)
		testutil.Exec(t, db, "DROP TABLE tasks")
		got, err := r.Complete(context.Background(), 1)
		if got != (model.Task{}) || !errors.Is(err, model.ErrStorage) {
			t.Fatalf("complete=%#v,%v", got, err)
		}
	})
	t.Run("rollback_update", func(t *testing.T) {
		db, path := testutil.Fixture(t, true)
		testutil.Exec(t, db, "CREATE TRIGGER deny_update BEFORE UPDATE ON tasks BEGIN SELECT RAISE(ABORT,'denied'); END")
		got, err := repository.New(db).Complete(context.Background(), 1)
		if got != (model.Task{}) || !errors.Is(err, model.ErrStorage) {
			t.Fatalf("complete=%#v,%v", got, err)
		}
		testutil.Rows(t, testutil.Independent(t, path), []model.Task{{ID: 1, Title: "Отчет"}})
	})
	t.Run("scan_failure", func(t *testing.T) {
		db, _ := testutil.Fixture(t, false)
		testutil.Exec(t, db, "DROP TABLE tasks")
		testutil.Exec(t, db, "CREATE TABLE tasks(id INTEGER, title TEXT, done TEXT)")
		testutil.Exec(t, db, "INSERT INTO tasks VALUES(1,'A','not-bool')")
		got, err := repository.New(db).List(context.Background())
		if got != nil || !errors.Is(err, model.ErrStorage) {
			t.Fatalf("list=%#v,%v", got, err)
		}
	})
}
