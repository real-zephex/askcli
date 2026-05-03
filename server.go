package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

var sCTX context.Context
var sDB *sql.DB
var sLocalKey string

type AskRequest struct {
	Message   string `json:"message"`
	Mode      string `json:"mode"`
	Model     string `json:"model"`
	Reasoning string `json:"reasoning"`
}

type AskResponse struct {
	Response string `json:"response"`
	Error    string `json:"error"`
}

func getLocalServerKey() (string, error) {
	localKey, exists := os.LookupEnv("ASKCLI_SERVER_KEY")
	if !exists {
		return "", fmt.Errorf("no ASKCLI_SERVER_KEY found in environment")
	}
	return localKey, nil
}

func apiKeyCheckerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("x-askcli-api-key")
		if apiKey != sLocalKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(

		func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			fmt.Printf("Origin: %s \n[%s] %s %s\n", r.RemoteAddr, time.Now().Format("2006-01-02 15:04:05"), r.Method, r.RequestURI)

			body, _ := io.ReadAll(r.Body)
			fmt.Println(string(body))
			r.Body = io.NopCloser(bytes.NewBuffer(body))

			next.ServeHTTP(w, r)

			duration := time.Since(start)
			fmt.Printf("  └─ Completed in %v\n", duration)

		},
	)
}

func handleAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	sReasoning := resolveReasoningLevel(req.Reasoning)
	sModel := resolveModels(req.Model)
	sGeminiKey, exists := checkForEnv()
	if !exists {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(AskResponse{Error: "GEMINI_API_KEY does not exists on the server."})
		return
	}

	//INFO: 0 is TELEGRAM CHAT ID. This will not support TELEGRAM RELATED FUNCTIONS so we need to exclude them.
	res := runAgentTurn(sCTX, sDB, sGeminiKey, req.Message, sModel, sReasoning, true, 0)

	//TODO Save user message and history to local database
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AskResponse{Response: res})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func startServer(db *sql.DB, ctx context.Context) {
	mux := http.NewServeMux()
	sCTX = ctx
	sDB = db

	localKey, err := getLocalServerKey()
	if err != nil {
		fmt.Println("No ASKCLI_SERVER_KEY found in environment variable. Not proceeding with booting the server")
		return
	}
	sLocalKey = localKey

	mux.HandleFunc("/ask", handleAsk)
	mux.HandleFunc("/health", handleHealth) // no auth on this one

	// wrap everything except /health in auth
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			mux.ServeHTTP(w, r)
			return
		}
		loggerMiddleware(apiKeyCheckerMiddleware(mux)).ServeHTTP(w, r)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 120 * time.Second, // agent runs can be slow
		IdleTimeout:  60 * time.Second,
	}

	fmt.Println("Server running on port:", port)
	if err := server.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
