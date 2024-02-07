package normalize

import "time"

type Quote struct {
	Symbol        string    `json:"symbol"`
	FromSymbol    string    `json:"fromSymbol"`
	Price         float64   `json:"price"`
	LastRefreshed time.Time `json:"refreshedAt"`
}
