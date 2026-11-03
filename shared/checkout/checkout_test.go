package checkout

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func TestTooManyLines(t *testing.T) {
	line := LineItem{Drink: DrinkCoffee, Size: SizeS, Quantity: 1}
	if got := Validate(Order{Items: slices.Repeat([]LineItem{line}, MaxLinesPerOrder)}); got != nil {
		t.Errorf("a full order was rejected: %+v", got)
	}
	got := Validate(Order{Items: slices.Repeat([]LineItem{line}, MaxLinesPerOrder+1)})
	if len(got) != 1 || got[0].Path != "items" {
		t.Errorf("violations = %+v", got)
	}
}

func TestMenuCannotBeChangedByCallers(t *testing.T) {
	Menu()[0].Price = 1
	if Menu()[0].Price == 1 {
		t.Error("changing the result of Menu changed the menu")
	}
}

// The same file runs against the Wasm build in tests/wasm.test.ts.
func TestCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name           string   `json:"name"`
		Order          Order    `json:"order"`
		Quote          *Quote   `json:"quote"`
		ViolationPaths []string `json:"violationPaths"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			paths := []string{}
			for _, v := range Validate(c.Order) {
				paths = append(paths, v.Path)
			}
			if !slices.Equal(paths, c.ViolationPaths) {
				t.Errorf("violations = %v, want %v", paths, c.ViolationPaths)
			}

			quote, err := Price(c.Order)
			if c.Quote == nil {
				if err == nil {
					t.Errorf("Price succeeded with %+v, want an error", quote)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if quote != *c.Quote {
				t.Errorf("quote = %+v, want %+v", quote, *c.Quote)
			}
		})
	}
}
