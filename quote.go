package normalize

import "time"

type Symbol string

type Quote struct {
	Symbol        Symbol    `json:"symbol"`
	FromSymbol    Symbol    `json:"fromSymbol"`
	Price         float64   `json:"price"`
	LastRefreshed time.Time `json:"refreshedAt"`
}
