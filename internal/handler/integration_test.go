package handler_test

import (
	"database/sql"
	"encoding/json"
	"example.com/tasks/internal/handler"
	"example.com/tasks/internal/model"
	"example.com/tasks/internal/repository"
	"example.com/tasks/internal/service"
	"example.com/tasks/internal/testutil"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func stack(t *testing.T, seed bool) (*handler.Handler, *sql.DB, string) {
	t.Helper()
	db, path := testutil.Fixture(t, seed)
	return handler.New(service.New(repository.New(db))), db, path
}
func request(h http.Handler, method, path, body, contentType string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func jsonRequest(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	return request(h, method, path, body, "application/json")
}
func TestI03HTTPTaskLifecycle(t *testing.T) {
	h, _, path := stack(t, false)
	check := testutil.Independent(t, path)
	a := model.Task{ID: 1, Title: "Отчет"}
	b := a
	b.Done = true
	testutil.Response(t, jsonRequest(h, "POST", "/api/tasks", `{"title":"Отчет"}`), 201, a)
	testutil.Response(t, jsonRequest(h, "GET", "/api/tasks", ""), 200, []model.Task{a})
	testutil.Response(t, jsonRequest(h, "PATCH", "/api/tasks/1/done", ""), 200, b)
	testutil.Response(t, jsonRequest(h, "DELETE", "/api/tasks/1", ""), 204, nil)
	testutil.Response(t, jsonRequest(h, "GET", "/api/tasks", ""), 200, []model.Task{})
	testutil.Rows(t, check, []model.Task{})
}
func TestI04HTTPInputFailures(t *testing.T) {
	type testCase struct {
		name, method, path, body, media string
		status                          int
		code, allow                     string
	}
	cases := []testCase{
		{"empty", "POST", "/api/tasks", `{"title":""}`, "application/json", 400, "title_length", ""},
		{"too_long", "POST", "/api/tasks", `{"title":"` + strings.Repeat("A", 101) + `"}`, "application/json", 400, "title_length", ""},
		{"malformed", "POST", "/api/tasks", `{"title":}`, "application/json", 400, "bad_json", ""},
		{"null_title", "POST", "/api/tasks", `{"title":null}`, "application/json", 400, "bad_json", ""},
		{"number", "POST", "/api/tasks", `{"title":7}`, "application/json", 400, "bad_json", ""},
		{"missing", "POST", "/api/tasks", `{}`, "application/json", 400, "bad_json", ""},
		{"unknown", "POST", "/api/tasks", `{"title":"А","extra":1}`, "application/json", 400, "bad_json", ""},
		{"second", "POST", "/api/tasks", `{"title":"А"} {}`, "application/json", 400, "bad_json", ""},
		{"media", "POST", "/api/tasks", `{"title":"А"}`, "text/plain", 415, "media_type", ""},
		{"missing_id", "PATCH", "/api/tasks/9/done", "", "", 404, "not_found", ""},
		{"method", "PUT", "/api/tasks", "", "", 405, "method", "GET, POST"},
		{"route", "GET", "/api/unknown", "", "", 404, "route", ""},
	}
	for _, id := range []string{"0", "-1", "01", "abc", "2147483648"} {
		cases = append(cases, testCase{"id_" + id, "PATCH", "/api/tasks/" + id + "/done", "", "", 400, "bad_id", ""})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _, path := stack(t, true)
			w := request(h, tc.method, tc.path, tc.body, tc.media)
			testutil.Response(t, w, tc.status, map[string]string{"error": tc.code})
			if got := w.Header().Get("Allow"); got != tc.allow {
				t.Fatalf("Allow=%q want=%q", got, tc.allow)
			}
			testutil.Rows(t, testutil.Independent(t, path), []model.Task{{ID: 1, Title: "Отчет"}})
		})
	}
}
func TestI05ClosedDatabase(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/tasks", ""}, {"POST", "/api/tasks", `{"title":"Отчет"}`},
		{"PATCH", "/api/tasks/1/done", ""}, {"DELETE", "/api/tasks/1", ""},
	} {
		t.Run(tc.method, func(t *testing.T) {
			h, db, path := stack(t, true)
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			w := jsonRequest(h, tc.method, tc.path, tc.body)
			testutil.Response(t, w, 500, map[string]string{"error": "storage"})
			for _, secret := range []string{path, "SELECT", "INSERT", "UPDATE", "DELETE", "database is closed"} {
				if strings.Contains(w.Body.String(), secret) {
					t.Fatalf("leak %q", secret)
				}
			}
			testutil.Rows(t, testutil.Independent(t, path), []model.Task{{ID: 1, Title: "Отчет"}})
		})
	}
}
func TestS03SQLInjection(t *testing.T) {
	h, _, path := stack(t, true)
	payload := "x'); DROP TABLE tasks;--"
	body, err := json.Marshal(map[string]string{"title": payload})
	if err != nil {
		t.Fatal(err)
	}
	injected := model.Task{ID: 2, Title: payload}
	testutil.Response(t, jsonRequest(h, "POST", "/api/tasks", string(body)), 201, injected)
	want := []model.Task{{ID: 1, Title: "Отчет"}, injected}
	testutil.Response(t, jsonRequest(h, "GET", "/api/tasks", ""), 200, want)
	check := testutil.Independent(t, path)
	testutil.Rows(t, check, want)
	if n := testutil.Scalar(t, check, "SELECT COUNT(*) FROM tasks"); n != 2 {
		t.Fatalf("count=%d", n)
	}
	var title string
	if err := check.QueryRow("SELECT title FROM tasks WHERE id=2").Scan(&title); err != nil || title != payload {
		t.Fatalf("title=%q err=%v", title, err)
	}
}
func TestI06TransportEdges(t *testing.T) {
	t.Run("html", func(t *testing.T) {
		h, _, _ := stack(t, true)
		w := request(h, "GET", "/", "", "")
		if w.Code != 200 || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
			t.Fatalf("HTTP=%d %v", w.Code, w.Header())
		}
		if !strings.Contains(w.Body.String(), `class="title">Отчет</td>`) {
			t.Fatal("server-rendered title missing")
		}
	})
	for i, tc := range []struct {
		method, path, body, media string
		status                    int
		code, allow               string
	}{
		{"POST", "/", "", "", 405, "method", "GET"},
		{"POST", "/api/tasks/0/done", `{`, "text/plain", 405, "method", "PATCH"},
		{"PATCH", "/api/tasks/1", "", "", 405, "method", "DELETE"},
		{"GET", "/api/tasks/1/extra", "", "", 404, "route", ""},
		{"GET", "/api/tasks/", "", "", 404, "route", ""},
		{"GET", "/api/tasks//done", "", "", 404, "route", ""},
		{"POST", "/api/tasks", `{"title":"A"}`, "", 415, "media_type", ""},
		{"POST", "/api/tasks", `{"title":"A"}`, "application/json; charset=latin1", 415, "media_type", ""},
		{"POST", "/api/tasks", `{"title":"A"}`, "application/json; other=x", 415, "media_type", ""},
		{"POST", "/api/tasks", `{"title":"A"}`, "application/json; charset=utf-8; x=y", 415, "media_type", ""},
		{"POST", "/api/tasks", `null`, "application/json", 400, "bad_json", ""},
		{"POST", "/api/tasks", `[]`, "application/json", 400, "bad_json", ""},
		{"POST", "/api/tasks", `{"title":"A"} broken`, "application/json", 400, "bad_json", ""},
		{"DELETE", "/api/tasks/9", "", "", 404, "not_found", ""},
		{"DELETE", "/api/tasks/999999999999999999999", "", "", 400, "bad_id", ""},
		{"DELETE", "/api/tasks/1x", "", "", 400, "bad_id", ""},
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			h, _, _ := stack(t, false)
			w := request(h, tc.method, tc.path, tc.body, tc.media)
			testutil.Response(t, w, tc.status, map[string]string{"error": tc.code})
			if w.Header().Get("Allow") != tc.allow {
				t.Fatal("Allow")
			}
		})
	}
	t.Run("charset_utf8", func(t *testing.T) {
		h, _, _ := stack(t, false)
		testutil.Response(t, request(h, "POST", "/api/tasks", `{"title":"Я"}`, "application/json; charset=utf-8"), 201, model.Task{ID: 1, Title: "Я"})
	})
	t.Run("page_storage", func(t *testing.T) {
		h, db, _ := stack(t, false)
		db.Close()
		testutil.Response(t, request(h, "GET", "/", "", ""), 500, map[string]string{"error": "storage"})
	})
}

func TestI08ConcurrentHTTP(t *testing.T) {
	h, _, path := stack(t, false)
	server := httptest.NewServer(h)
	defer server.Close()
	type result struct {
		task model.Task
		err  error
	}
	results := make(chan result, 20)
	var group sync.WaitGroup
	for i := 0; i < 20; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			title := fmt.Sprintf("Параллельно-%02d", i)
			encoded, err := json.Marshal(map[string]string{"title": title})
			if err != nil {
				results <- result{err: err}
				return
			}
			response, err := server.Client().Post(server.URL+"/api/tasks", "application/json", strings.NewReader(string(encoded)))
			if err != nil {
				results <- result{err: err}
				return
			}
			defer response.Body.Close()
			if response.StatusCode != 201 {
				results <- result{err: fmt.Errorf("HTTP %d", response.StatusCode)}
				return
			}
			var task model.Task
			if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
				results <- result{err: err}
				return
			}
			if task.Title != title || task.Done || task.ID < 1 || task.ID > 20 {
				results <- result{err: fmt.Errorf("task %#v", task)}
				return
			}
			results <- result{task: task}
		}(i)
	}
	group.Wait()
	close(results)
	seen := map[int64]model.Task{}
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if _, exists := seen[result.task.ID]; exists {
			t.Fatalf("duplicate id %d", result.task.ID)
		}
		seen[result.task.ID] = result.task
	}
	if len(seen) != 20 {
		t.Fatalf("count=%d", len(seen))
	}
	want := []model.Task{}
	for id := int64(1); id <= 20; id++ {
		value, exists := seen[id]
		if !exists {
			t.Fatalf("missing id %d", id)
		}
		want = append(want, value)
	}
	testutil.Response(t, jsonRequest(h, "GET", "/api/tasks", ""), 200, want)
	testutil.Rows(t, testutil.Independent(t, path), want)
}
