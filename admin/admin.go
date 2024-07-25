package admin

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

func HTTPHandler(logLevel *slog.LevelVar) http.Handler {
	router := mux.NewRouter()
	router.PathPrefix("/admin").Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verbosity := r.URL.Query().Get("verbosity")
		switch verbosity {
		case "debug":
			logLevel.Set(slog.LevelDebug)
		case "info":
			logLevel.Set(slog.LevelInfo)
		case "warn":
			logLevel.Set(slog.LevelWarn)
		case "error":
			logLevel.Set(slog.LevelError)
		default:
			http.Error(w, "Invalid verbosity level", http.StatusBadRequest)
			return
		}

		fmt.Fprintln(w, "Verbosity changed to ", verbosity)
	}))
	return handlers.CompressHandler(router)
}
