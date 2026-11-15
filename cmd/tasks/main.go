package main

import (
	"context"
	"errors"
	"example.com/tasks/internal/handler"
	"example.com/tasks/internal/repository"
	"example.com/tasks/internal/service"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	db, err := repository.Open(env("DB_PATH", "tasks.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	server := &http.Server{
		Addr:              env("ADDR", "127.0.0.1:8080"),
		Handler:           handler.New(service.New(repository.New(db))),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
		close(shutdownDone)
	}()
	log.Printf("tasks listening on http://%s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-shutdownDone
}
