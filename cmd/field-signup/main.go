package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/example/fieldservice-email-cutover/internal/infrai"
	"github.com/example/fieldservice-email-cutover/internal/signup"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	publicURL := envOr("PUBLIC_URL", "http://localhost:8080")
	client := &infrai.EmailClient{BaseURL: infrai.DefaultBaseURL, APIKey: key, MaxRetries: 3}
	flow := signup.NewFlow(client, publicURL)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /signup", func(w http.ResponseWriter, r *http.Request) {
		var input signup.Registration
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		result, err := flow.Register(r.Context(), input)
		if err != nil {
			var apiErr *infrai.APIError
			if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
				writeJSON(w, apiErr.StatusCode, map[string]string{"error": apiErr.Message})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "verification email could not be sent"})
			return
		}
		writeJSON(w, http.StatusCreated, result)
	})
	mux.HandleFunc("GET /verify", func(w http.ResponseWriter, r *http.Request) {
		result, err := flow.Verify(r.URL.Query().Get("token"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	log.Printf("field signup listening on %s", strings.TrimPrefix(publicURL, "http://"))
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return strings.TrimRight(value, "/")
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
