// Command server is the backend. It calls the shared rules as ordinary Go, and
// never trusts the total the frontend computed with the same rules.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"

	"github.com/super-himetan/go-ts-shared-logic/shared/checkout"
)

const addr = "localhost:8787"

func main() {
	mux := newMux()
	mux.Handle("GET /", staticHandler(os.DirFS("dist")))
	log.Printf("listening on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func newMux() *http.ServeMux {
	var lastID atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/orders", func(w http.ResponseWriter, r *http.Request) {
		var req checkout.PlaceOrderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
				Message: fmt.Sprintf("金額が変わりました: 表示 %d円 / 実際 %d円", req.ExpectedTotal, quote.Total),
			})
			return
		}
		writeJSON(w, http.StatusCreated, checkout.PlaceOrderResponse{
			OrderID: fmt.Sprintf("order-%d", lastID.Add(1)),
			Quote:   quote,
		})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Print(err)
	}
}
