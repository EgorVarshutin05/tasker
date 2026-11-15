package model

import "errors"

type Task struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

var (
	ErrTitle    = errors.New("title_length")
	ErrID       = errors.New("bad_id")
	ErrNotFound = errors.New("not_found")
	ErrStorage  = errors.New("storage")
)
