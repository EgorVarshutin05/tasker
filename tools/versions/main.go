package main

import (
	"encoding/json"
	"github.com/mattn/go-sqlite3"
	"os"
	"runtime"
)

func main() {
	version, number, source := sqlite3.Version()
	json.NewEncoder(os.Stdout).Encode(map[string]any{
		"go": runtime.Version(), "sqlite": version, "sqlite_number": number, "sqlite_source_id": source,
	})
}
