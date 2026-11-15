package service_test

import (
	"context"
	"errors"
	"example.com/tasks/internal/model"
	"example.com/tasks/internal/service"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type repoStub struct {
	calls map[string]int
	title string
	id    int64
	task  model.Task
	tasks []model.Task
	err   error
}

func stub() *repoStub { return &repoStub{calls: map[string]int{}} }
func (r *repoStub) Create(_ context.Context, title string) (model.Task, error) {
	r.calls["Create"]++
	r.title = title
	return r.task, r.err
}
func (r *repoStub) List(context.Context) ([]model.Task, error) {
	r.calls["List"]++
	return r.tasks, r.err
}
func (r *repoStub) Complete(_ context.Context, id int64) (model.Task, error) {
	r.calls["Complete"]++
	r.id = id
	return r.task, r.err
}
func (r *repoStub) Delete(_ context.Context, id int64) error {
	r.calls["Delete"]++
	r.id = id
	return r.err
}
func calls(t *testing.T, r *repoStub, name string, count int) {
	t.Helper()
	for _, method := range []string{"Create", "List", "Complete", "Delete"} {
		want := 0
		if method == name {
			want = count
		}
		if r.calls[method] != want {
			t.Fatalf("%s calls=%d want=%d", method, r.calls[method], want)
		}
	}
}
func task(t *testing.T, got, want model.Task, err, wantErr error) {
	t.Helper()
	if got != want || !errors.Is(err, wantErr) {
		t.Fatalf("got (%#v,%v), want (%#v,%v)", got, err, want, wantErr)
	}
}
func TestU01Create(t *testing.T) {
	r := stub()
	r.task = model.Task{ID: 1, Title: "Отчет", Done: false}
	got, err := service.New(r).Create(context.Background(), "  Отчет  ")
	task(t, got, r.task, err, nil)
	calls(t, r, "Create", 1)
	if r.title != "Отчет" {
		t.Fatalf("title=%q", r.title)
	}
}
func TestU02TitleBoundaries(t *testing.T) {
	cases := []struct {
		name, input, stored string
		bad                 bool
	}{
		{"lower", "Я", "Я", false},
		{"upper", strings.Repeat("A", 100), strings.Repeat("A", 100), false},
		{"empty", "", "", true}, {"spaces", "   ", "", true},
		{"too_long", strings.Repeat("A", 101), "", true},
		{"bad_utf8", string([]byte{0xff}), "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := stub()
			r.task = model.Task{ID: 1, Title: tc.stored}
			got, err := service.New(r).Create(context.Background(), tc.input)
			if tc.bad {
				task(t, got, model.Task{}, err, model.ErrTitle)
				calls(t, r, "Create", 0)
			} else {
				task(t, got, r.task, err, nil)
				calls(t, r, "Create", 1)
			}
			if r.title != tc.stored {
				t.Fatalf("argument=%q want=%q", r.title, tc.stored)
			}
		})
	}
}
func TestU03IDBoundaries(t *testing.T) {
	for _, method := range []string{"Complete", "Delete"} {
		for _, id := range []int64{1, 2147483647, 0, -1, 2147483648} {
			t.Run(fmt.Sprintf("%s/%d", method, id), func(t *testing.T) {
				r := stub()
				r.task = model.Task{ID: id, Title: "Отчет", Done: true}
				svc := service.New(r)
				valid := id >= 1 && id <= 2147483647
				var err error
				if method == "Complete" {
					var got model.Task
					got, err = svc.Complete(context.Background(), id)
					want := model.Task{}
					if valid {
						want = r.task
					}
					wantErr := model.ErrID
					if valid {
						wantErr = nil
					}
					task(t, got, want, err, wantErr)
				} else {
					err = svc.Delete(context.Background(), id)
					wantErr := model.ErrID
					if valid {
						wantErr = nil
					}
					if !errors.Is(err, wantErr) {
						t.Fatalf("error=%v want=%v", err, wantErr)
					}
				}
				n := 0
				if valid {
					n = 1
					if r.id != id {
						t.Fatalf("id argument=%d", r.id)
					}
				}
				calls(t, r, method, n)
			})
		}
	}
}
func TestU04List(t *testing.T) {
	rows := []model.Task{{ID: 1, Title: "А"}, {ID: 2, Title: "Б", Done: true}}
	for _, tc := range []struct {
		name        string
		input, want []model.Task
	}{
		{"ordered", rows, rows}, {"empty", []model.Task{}, []model.Task{}}, {"nil", nil, []model.Task{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := stub()
			r.tasks = tc.input
			got, err := service.New(r).List(context.Background())
			if err != nil || got == nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got (%#v,%v)", got, err)
			}
			calls(t, r, "List", 1)
		})
	}
}
func TestU05RepositoryErrors(t *testing.T) {
	for _, method := range []string{"Create", "List", "Complete", "Delete"} {
		failures := []error{model.ErrStorage}
		if method == "Complete" || method == "Delete" {
			failures = append(failures, model.ErrNotFound)
		}
		for _, failure := range failures {
			t.Run(method+"/"+failure.Error(), func(t *testing.T) {
				r := stub()
				r.err = failure
				svc := service.New(r)
				ctx := context.Background()
				var err error
				switch method {
				case "Create":
					var got model.Task
					got, err = svc.Create(ctx, "Отчет")
					task(t, got, model.Task{}, err, failure)
				case "List":
					var got []model.Task
					got, err = svc.List(ctx)
					if got != nil {
						t.Fatalf("list=%#v", got)
					}
				case "Complete":
					var got model.Task
					got, err = svc.Complete(ctx, 9)
					task(t, got, model.Task{}, err, failure)
				case "Delete":
					err = svc.Delete(ctx, 9)
				}
				if !errors.Is(err, failure) {
					t.Fatalf("error=%v want=%v", err, failure)
				}
				calls(t, r, method, 1)
				if method == "Create" && r.title != "Отчет" {
					t.Fatalf("title=%q", r.title)
				}
				if (method == "Complete" || method == "Delete") && r.id != 9 {
					t.Fatalf("id=%d", r.id)
				}
			})
		}
	}
}
