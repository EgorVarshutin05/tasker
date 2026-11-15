// Package testutil provides isolated fixtures; it is not linked into the server.
package testutil

import (
	"database/sql"
	"encoding/json"
	"example.com/tasks/internal/model"
	"example.com/tasks/internal/repository"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
)

func Fixture(t *testing.T, seed bool) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tasks.db")
	db, err := repository.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if seed {
		Exec(t, db, "INSERT INTO tasks(id,title,done) VALUES(1,?,0)", "Отчет")
	}
	return db, path
}
func Independent(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	Exec(t, db, "PRAGMA busy_timeout=5000")
	return db
}
func Exec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}
func Scalar(t *testing.T, db *sql.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func Rows(t *testing.T, db *sql.DB, want []model.Task) {
	t.Helper()
	rows, err := db.Query("SELECT id,title,done FROM tasks ORDER BY id ASC")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := []model.Task{}
	for rows.Next() {
		var v model.Task
		if err := rows.Scan(&v.ID, &v.Title, &v.Done); err != nil {
			t.Fatal(err)
		}
		got = append(got, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SQL rows=%#v want=%#v", got, want)
	}
}
func Response(t *testing.T, r *httptest.ResponseRecorder, status int, want any) {
	t.Helper()
	if r.Code != status {
		t.Fatalf("HTTP=%d want=%d body=%s", r.Code, status, r.Body.String())
	}
	if status == 204 {
		if r.Body.Len() != 0 {
			t.Fatalf("204 body=%q", r.Body.String())
		}
		return
	}
	if got := r.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type=%q", got)
	}
	var got, expected any
	if err := json.Unmarshal(r.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("JSON=%#v want=%#v", got, expected)
	}
}
