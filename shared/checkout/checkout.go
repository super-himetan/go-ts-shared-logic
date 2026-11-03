// Package checkout is the one implementation of the shop's pricing rules.
// The backend calls it natively, and the frontend imports this file directly.
package checkout

import (
	"errors"
	"fmt"
	"slices"
)

// Drink is a product on the menu.
type Drink string

const (
	DrinkCoffee Drink = "coffee"
	DrinkLatte  Drink = "latte"
	DrinkMatcha Drink = "matcha"
)

// Size is a cup size.
type Size string

const (
	SizeS Size = "S"
	SizeM Size = "M"
	SizeL Size = "L"
)

// MaxQuantityPerLine is the most cups one line of an order can hold.
const MaxQuantityPerLine = 9

// MaxLinesPerOrder is the most lines one order can hold.
const MaxLinesPerOrder = 20

// TaxPercent is the consumption tax rate.
const TaxPercent = 10

// MenuEntry is a drink with its display name and base price in yen.
type MenuEntry struct {
	Drink Drink  `json:"drink"`
	Name  string `json:"name"`
	Price int    `json:"price"`
}

// LineItem is one line of an order.
type LineItem struct {
	Drink    Drink `json:"drink"`
	Size     Size  `json:"size"`
	Quantity int   `json:"quantity"`
}

// Order is what a customer is about to buy.
type Order struct {
	Items  []LineItem `json:"items"`
	Coupon string     `json:"coupon,omitempty"`
}

// Violation says what is wrong with an order, and where.
type Violation struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Quote is the amount to pay, in yen.
type Quote struct {
	Subtotal int `json:"subtotal"`
	Discount int `json:"discount"`
	Tax      int `json:"tax"`
	Total    int `json:"total"`
}

// PlaceOrderRequest is the body of POST /api/orders.
type PlaceOrderRequest struct {
	Order Order `json:"order"`
	// ExpectedTotal is the total the customer saw, so a price change is never charged silently.
	ExpectedTotal int `json:"expectedTotal"`
}

// PlaceOrderResponse is the body of a successful POST /api/orders.
type PlaceOrderResponse struct {
	OrderID string `json:"orderId"`
	Quote   Quote  `json:"quote"`
}

// ErrorResponse is the body of a failed POST /api/orders.
type ErrorResponse struct {
	Message    string      `json:"message"`
	Violations []Violation `json:"violations"`
}

var menu = []MenuEntry{
	{Drink: DrinkCoffee, Name: "コーヒー", Price: 380},
	{Drink: DrinkLatte, Name: "カフェラテ", Price: 450},
	{Drink: DrinkMatcha, Name: "抹茶ラテ", Price: 520},
}

var sizeSurcharge = map[Size]int{SizeS: 0, SizeM: 50, SizeL: 110}

// couponPercent maps a coupon code to its discount rate.
var couponPercent = map[string]int{"WELCOME10": 10, "STAFF30": 30}

// Menu returns everything that can be ordered.
func Menu() []MenuEntry {
	return slices.Clone(menu)
}

// Validate returns every problem with the order, or nil if it can be placed.
func Validate(order Order) []Violation {
	var violations []Violation
	add := func(path, format string, args ...any) {
		violations = append(violations, Violation{Path: path, Message: fmt.Sprintf(format, args...)})
	}

	if len(order.Items) == 0 {
		add("items", "商品を1つ以上選んでください")
	}
	if len(order.Items) > MaxLinesPerOrder {
		add("items", "1回の注文は%d行までです", MaxLinesPerOrder)
	}
	for i, item := range order.Items {
		if _, ok := basePrice(item.Drink); !ok {
			add(fmt.Sprintf("items[%d].drink", i), "メニューにない商品です: %s", item.Drink)
		}
		if _, ok := sizeSurcharge[item.Size]; !ok {
			add(fmt.Sprintf("items[%d].size", i), "選べないサイズです: %s", item.Size)
		}
		if item.Quantity < 1 || item.Quantity > MaxQuantityPerLine {
			add(fmt.Sprintf("items[%d].quantity", i), "数量は1〜%dで指定してください", MaxQuantityPerLine)
		}
	}
	if order.Coupon != "" {
		if _, ok := couponPercent[order.Coupon]; !ok {
			add("coupon", "使えないクーポンです: %s", order.Coupon)
		}
	}
	return violations
}

// Price computes the amount to pay. The discount and the tax both round down.
func Price(order Order) (Quote, error) {
	if len(Validate(order)) > 0 {
		return Quote{}, errors.New("注文に問題があるため金額を計算できません")
	}

	subtotal := 0
	for _, item := range order.Items {
		price, _ := basePrice(item.Drink)
		subtotal += (price + sizeSurcharge[item.Size]) * item.Quantity
	}
	discount := subtotal * couponPercent[order.Coupon] / 100
	tax := (subtotal - discount) * TaxPercent / 100
	return Quote{
		Subtotal: subtotal,
		Discount: discount,
		Tax:      tax,
		Total:    subtotal - discount + tax,
	}, nil
}

func basePrice(drink Drink) (int, bool) {
	for _, entry := range menu {
		if entry.Drink == drink {
			return entry.Price, true
		}
	}
	return 0, false
}
