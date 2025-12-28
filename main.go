package main

import (
	"database/sql"
	"fmt"
	"go-finance/internal/handler"
	"go-finance/internal/store"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	httpSwagger "github.com/swaggo/http-swagger"

	_ "go-finance/docs"
)

// @title           ChatBot Finance API
// @version         1.0
// @description     API Server quản lý thu chi cá nhân cho Telegram Bot.
// @BasePath        /
// @schemes   https http
func main() {
	_ = godotenv.Load()

	// 1. Kết nối DB
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		log.Fatal("DATABASE_URL is required")
	}
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal("Cannot connect to DB:", err)
	}
	fmt.Println("Connected to Database successfully!")

	// 2. Init Store & Handler
	pgStore := store.NewPostgresStore(db)
	if err := pgStore.InitSchema(); err != nil {
		log.Fatal("Failed to init schema:", err)
	}

	h := handler.NewFinanceHandler(pgStore)

	// 3. Router
	mux := http.NewServeMux()
	mux.HandleFunc("POST /transactions", h.CreateTransaction) // RESTful style
	mux.HandleFunc("GET /report", h.GenerateReport)
	mux.HandleFunc("GET /market-rates", h.GetPrices)
	mux.HandleFunc("GET /users", h.GetUsers)

	mux.HandleFunc("/swagger/", httpSwagger.Handler(
		httpSwagger.URL("doc.json"),
	))

	// 4. Start Server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Server running on port " + port)
	http.ListenAndServe(":"+port, enableCORS(mux))
}

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
