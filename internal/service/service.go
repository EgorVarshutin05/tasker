package service

import (
	"context"
	"example.com/tasks/internal/model"
	"strings"
	"unicode/utf8"
)

type Repository interface {
	Create(context.Context, string) (model.Task, error)
	List(context.Context) ([]model.Task, error)
	Complete(context.Context, int64) (model.Task, error)
	Delete(context.Context, int64) error
}

type Service struct{ repo Repository }

func New(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Create(ctx context.Context, title string) (model.Task, error) {
	if !utf8.ValidString(title) {
		return model.Task{}, model.ErrTitle
	}
	title = strings.TrimSpace(title)
	n := utf8.RuneCountInString(title)
	if n < 1 || n > 100 {
		return model.Task{}, model.ErrTitle
	}
	task, err := s.repo.Create(ctx, title)
	if err != nil {
		return model.Task{}, err
	}
	return task, nil
}

func (s *Service) List(ctx context.Context) ([]model.Task, error) {
	tasks, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	if tasks == nil {
		tasks = []model.Task{}
	}
	return tasks, nil
}

func (s *Service) Complete(ctx context.Context, id int64) (model.Task, error) {
	if id < 1 || id > 2147483647 {
		return model.Task{}, model.ErrID
	}
	task, err := s.repo.Complete(ctx, id)
	if err != nil {
		return model.Task{}, err
	}
	return task, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id < 1 || id > 2147483647 {
		return model.ErrID
	}
	return s.repo.Delete(ctx, id)
}
