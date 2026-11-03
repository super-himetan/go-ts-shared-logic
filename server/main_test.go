package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestPlaceOrder(t *testing.T) {
	const coffee = `{"items":[{"drink":"coffee","size":"S","quantity":1}]}`
	tests := []struct {
		name, body string
		status     int
		contains   string
	}{
		{"accepted", `{"order":` + coffee + `,"expectedTotal":418}`, http.StatusCreated, `"orderId":"order-1"`},
		{"tampered total", `{"order":` + coffee + `,"expectedTotal":1}`, http.StatusConflict, "実際 418円"},
		{"invalid order", `{"order":{"items":[]},"expectedTotal":0}`, http.StatusUnprocessableEntity, `"path":"items"`},
		{"not json", `{`, http.StatusBadRequest, "読み取れません"},
		{"too large", `{"order":{"coupon":"` + strings.Repeat("x", maxBodyBytes) + `"}}`, http.StatusRequestEntityTooLarge, "大きすぎます"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			newHandler(fstest.MapFS{}).ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Errorf("status = %d, want %d", w.Code, tt.status)
			}
			if !strings.Contains(w.Body.String(), tt.contains) {
				t.Errorf("body = %s, want it to contain %s", w.Body, tt.contains)
			}
		})
	}
}

func TestCrossSiteOrdersAreRefused(t *testing.T) {
	tests := []struct {
		site   string
		status int
	}{
		{"cross-site", http.StatusForbidden},
		{"same-origin", http.StatusBadRequest}, // reaches the handler, which rejects the empty body
	}
	for _, tt := range tests {
		t.Run(tt.site, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader("{"))
			r.Header.Set("Sec-Fetch-Site", tt.site)
			w := httptest.NewRecorder()
			newHandler(fstest.MapFS{}).ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Errorf("status = %d, want %d", w.Code, tt.status)
			}
		})
	}
}
