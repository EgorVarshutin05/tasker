package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"example.com/tasks/internal/model"
	"example.com/tasks/web"
	"html/template"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

type TaskService interface {
	Create(context.Context, string) (model.Task, error)
	List(context.Context) ([]model.Task, error)
	Complete(context.Context, int64) (model.Task, error)
	Delete(context.Context, int64) error
}

type Handler struct {
	service TaskService
	page    *template.Template
}

func New(service TaskService) *Handler {
	return &Handler{service: service,
		page: template.Must(template.New("index").Parse(web.Index))}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func failure(w http.ResponseWriter, err error) {
	code, status := "storage", http.StatusInternalServerError
	switch {
	case errors.Is(err, model.ErrTitle):
		code, status = "title_length", 400
	case errors.Is(err, model.ErrID):
		code, status = "bad_id", 400
	case errors.Is(err, model.ErrNotFound):
		code, status = "not_found", 404
	}
	writeJSON(w, status, map[string]string{"error": code})
}

func method(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeJSON(w, 405, map[string]string{"error": "method"})
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		if r.Method != "GET" {
			method(w, "GET")
			return
		}
		tasks, err := h.service.List(r.Context())
		if err != nil {
			failure(w, err)
			return
		}
		var body bytes.Buffer
		if h.page.Execute(&body, tasks) != nil {
			failure(w, model.ErrStorage)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		w.Write(body.Bytes())
		return
	}
	if path == "/api/tasks" {
		if r.Method != "GET" && r.Method != "POST" {
			method(w, "GET, POST")
			return
		}
		if r.Method == "GET" {
			tasks, err := h.service.List(r.Context())
			if err != nil {
				failure(w, err)
				return
			}
			writeJSON(w, 200, tasks)
			return
		}
		media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" || len(params) > 1 ||
			(len(params) == 1 && !strings.EqualFold(params["charset"], "utf-8")) {
			writeJSON(w, 415, map[string]string{"error": "media_type"})
			return
		}
		var body struct {
			Title *string `json:"title"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if dec.Decode(&body) != nil || body.Title == nil || dec.Decode(&struct{}{}) != io.EOF {
			writeJSON(w, 400, map[string]string{"error": "bad_json"})
			return
		}
		task, err := h.service.Create(r.Context(), *body.Title)
		if err != nil {
			failure(w, err)
			return
		}
		writeJSON(w, 201, task)
		return
	}
	if strings.HasPrefix(path, "/api/tasks/") {
		parts := strings.Split(strings.TrimPrefix(path, "/api/tasks/"), "/")
		complete := len(parts) == 2 && parts[1] == "done"
		if parts[0] != "" && (len(parts) == 1 || complete) {
			allow := "DELETE"
			if complete {
				allow = "PATCH"
			}
			if r.Method != allow {
				method(w, allow)
				return
			}
			id, err := parseID(parts[0])
			if err != nil {
				failure(w, err)
				return
			}
			if complete {
				task, err := h.service.Complete(r.Context(), id)
				if err != nil {
					failure(w, err)
					return
				}
				writeJSON(w, 200, task)
			} else {
				if err := h.service.Delete(r.Context(), id); err != nil {
					failure(w, err)
					return
				}
				w.WriteHeader(204)
			}
			return
		}
	}
	writeJSON(w, 404, map[string]string{"error": "route"})
}

func parseID(raw string) (int64, error) {
	if len(raw) == 0 || raw[0] < '1' || raw[0] > '9' {
		return 0, model.ErrID
	}
	for _, c := range raw {
		if c < '0' || c > '9' {
			return 0, model.ErrID
		}
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id > 2147483647 {
		return 0, model.ErrID
	}
	return id, nil
}
