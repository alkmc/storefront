package entity

import "testing"

func TestCurrency_Valid(t *testing.T) {
	tests := []struct {
		name     string
		currency Currency
		want     bool
	}{
		{name: "PLN", currency: CurrencyPLN, want: true},
		{name: "EUR", currency: CurrencyEUR, want: true},
		{name: "USD", currency: CurrencyUSD, want: true},
		{name: "GBP", currency: CurrencyGBP, want: true},
		{name: "CHF", currency: CurrencyCHF, want: true},
		{name: "unknown", currency: Currency("XXX"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.currency.Valid(); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMoney_Validate(t *testing.T) {
	tests := []struct {
		name    string
		money   Money
		wantErr bool
	}{
		{
			name:  "valid",
			money: Money{MinorAmount: 100, Currency: CurrencyPLN},
		},
		{
			name:    "zero amount",
			money:   Money{MinorAmount: 0, Currency: CurrencyPLN},
			wantErr: true,
		},
		{
			name:    "negative amount",
			money:   Money{MinorAmount: -1, Currency: CurrencyPLN},
			wantErr: true,
		},
		{
			name:    "invalid currency",
			money:   Money{MinorAmount: 100, Currency: Currency("XXX")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.money.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
