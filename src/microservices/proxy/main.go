package main

import (
	"encoding/json"
	"log"
	"math/rand/v2"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
)

func main() {
	port := getEnv("PORT", "8000")
	monolith := newProxy(getEnv("MONOLITH_URL", "http://monolith:8080"))
	movies := newProxy(getEnv("MOVIES_SERVICE_URL", "http://movies-service:8081"))
	events := newProxy(getEnv("EVENTS_SERVICE_URL", "http://events-service:8082"))

	// Фиче-флаг: включена ли миграция и какой процент запросов к /api/movies идёт в новый сервис
	gradualMigration := getEnv("GRADUAL_MIGRATION", "false") == "true"
	moviesPercent, _ := strconv.Atoi(getEnv("MOVIES_MIGRATION_PERCENT", "0"))

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"status": true})
	})

	// Strangler Fig: часть запросов к фильмам отправляем в movies-service, остальное в монолит
	mux.HandleFunc("/api/movies", func(w http.ResponseWriter, r *http.Request) {
		if gradualMigration && rand.IntN(100) < moviesPercent {
			log.Printf("%s %s -> movies-service", r.Method, r.URL)
			movies.ServeHTTP(w, r)
			return
		}
		log.Printf("%s %s -> monolith", r.Method, r.URL)
		monolith.ServeHTTP(w, r)
	})

	mux.Handle("/api/events/", events)

	// Всё, что ещё не вынесено из монолита
	mux.Handle("/", monolith)

	log.Printf("Proxy started on port %s, migration=%v, movies percent=%d", port, gradualMigration, moviesPercent)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func newProxy(target string) *httputil.ReverseProxy {
	u, err := url.Parse(target)
	if err != nil {
		log.Fatalf("bad url %q: %v", target, err)
	}
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(u)
			r.SetXForwarded()
		},
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
