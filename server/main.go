// Command server is the backend. It calls the shared rules as ordinary Go, and
// never trusts the total the frontend computed with the same rules.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/super-himetan/go-ts-shared-logic/shared/checkout"
)

const (
	addr = "localhost:8787"
	// maxBodyBytes easily holds the largest order that Validate accepts.
	maxBodyBytes = 64 << 10
)

func main() {
	log.Printf("listening on http://%s", addr)
	server := &http.Server{
		Addr:              addr,
		Handler:           newHandler(os.DirFS("dist")),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	log.Fatal(server.ListenAndServe())
}

func newHandler(files fs.FS) http.Handler {
	var lastID atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/orders", func(w http.ResponseWriter, r *http.Request) {
		var req checkout.PlaceOrderRequest
		body := http.MaxBytesReader(w, r.Body, maxBodyBytes)
		if err := json.NewDecoder(body).Decode(&req); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeJSON(w, http.StatusRequestEntityTooLarge, checkout.ErrorResponse{Message: "リクエストが大きすぎます"})
				return
			}
			writeJSON(w, http.StatusBadRequest, checkout.ErrorResponse{Message: "リクエストを読み取れません"})
			return
		}
		if violations := checkout.Validate(req.Order); len(violations) > 0 {
			writeJSON(w, http.StatusUnprocessableEntity, checkout.ErrorResponse{
				Message:    "注文に問題があります",
				Violations: violations,
			})
			return
		}
		quote, err := checkout.Price(req.Order)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, checkout.ErrorResponse{Message: err.Error()})
			return
		}
		if quote.Total != req.ExpectedTotal {
			writeJSON(w, http.StatusConflict, checkout.ErrorResponse{
				Message: fmt.Sprintf("金額が変わりました: 表示 %s / 実際 %s", checkout.FormatYen(req.ExpectedTotal), checkout.FormatYen(quote.Total)),
			})
			return
		}
		writeJSON(w, http.StatusCreated, checkout.PlaceOrderResponse{
			OrderID: fmt.Sprintf("order-%d", lastID.Add(1)),
			Quote:   quote,
		})
	})
	mux.Handle("GET /", staticHandler(files))
	// Another site could otherwise place an order from a visitor's browser.
	return http.NewCrossOriginProtection().Handler(mux)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Print(err)
	}
}
