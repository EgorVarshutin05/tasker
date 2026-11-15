package repository

import (
	"context"
	"database/sql"
	"example.com/tasks/internal/model"
	_ "github.com/mattn/go-sqlite3"
)

const schema = `CREATE TABLE IF NOT EXISTS tasks (
 id INTEGER PRIMARY KEY AUTOINCREMENT CHECK (id BETWEEN 1 AND 2147483647),
 title TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 100),
 done INTEGER NOT NULL DEFAULT 0 CHECK (done IN (0, 1))
)`

type SQLiteRepository struct{ db *sql.DB }

// Open initializes the schema once. Request handlers never call it.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, model.ErrStorage
	}
	db.SetMaxOpenConns(1)
	for _, query := range []string{
		"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=WAL", schema,
	} {
		if _, err = db.Exec(query); err != nil {
			db.Close()
			return nil, model.ErrStorage
		}
	}
	return db, nil
}

func New(db *sql.DB) *SQLiteRepository { return &SQLiteRepository{db: db} }

func (r *SQLiteRepository) Create(ctx context.Context, title string) (model.Task, error) {
	result, err := r.db.ExecContext(ctx,
		"INSERT INTO tasks(title, done) VALUES (?, 0)", title)
	if err != nil {
		return model.Task{}, model.ErrStorage
	}
	id, err := result.LastInsertId()
	if err != nil {
		return model.Task{}, model.ErrStorage
	}
	return model.Task{ID: id, Title: title, Done: false}, nil
}

func (r *SQLiteRepository) List(ctx context.Context) ([]model.Task, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, title, done FROM tasks ORDER BY id ASC")
	if err != nil {
		return nil, model.ErrStorage
	}
	defer rows.Close()
	tasks := []model.Task{}
	for rows.Next() {
		var task model.Task
		if err := rows.Scan(&task.ID, &task.Title, &task.Done); err != nil {
			return nil, model.ErrStorage
		}
		tasks = append(tasks, task)
	}
	if rows.Err() != nil {
		return nil, model.ErrStorage
	}
	return tasks, nil
}

func (r *SQLiteRepository) Complete(ctx context.Context, id int64) (model.Task, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Task{}, model.ErrStorage
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE tasks SET done = 1 WHERE id = ?", id)
	if err != nil {
		return model.Task{}, model.ErrStorage
	}
	n, err := result.RowsAffected()
	if err != nil {
		return model.Task{}, model.ErrStorage
	}
	if n == 0 {
		return model.Task{}, model.ErrNotFound
	}
	var task model.Task
	err = tx.QueryRowContext(ctx,
		"SELECT id, title, done FROM tasks WHERE id = ?", id).
		Scan(&task.ID, &task.Title, &task.Done)
	if err != nil {
		return model.Task{}, model.ErrStorage
	}
	if tx.Commit() != nil {
		return model.Task{}, model.ErrStorage
	}
	return task, nil
}

func (r *SQLiteRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return model.ErrStorage
	}
	n, err := result.RowsAffected()
	if err != nil {
		return model.ErrStorage
	}
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}
