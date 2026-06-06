package entity

import "errors"

// Currency is a supported ISO 4217 currency code.
type Currency string

const (
	// Keep this list in sync with the products.currency CHECK constraint.
	CurrencyPLN Currency = "PLN"
	CurrencyEUR Currency = "EUR"
	CurrencyUSD Currency = "USD"
	CurrencyGBP Currency = "GBP"
	CurrencyCHF Currency = "CHF"
)

// Money stores MinorAmount in the smallest unit of Currency, e.g. cents or grosze.
type Money struct {
	MinorAmount int64
	Currency    Currency
}

func (c Currency) Valid() bool {
	switch c {
	case CurrencyPLN, CurrencyEUR, CurrencyUSD, CurrencyGBP, CurrencyCHF:
		return true
	default:
		return false
	}
}

func (m Money) Validate() error {
	if m.MinorAmount <= 0 {
		return errors.New("the product price must be positive")
	}
	if !m.Currency.Valid() {
		return errors.New("the product currency is invalid")
	}
	return nil
}
