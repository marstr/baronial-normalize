package fetch

import (
	"context"
	"log"
	"time"

	normalize "github.com/marstr/baronial-normalize"
	"github.com/marstr/collection/v2"
)

type Cache struct {
	underlyer   *collection.LRUCache[normalize.Symbol, normalize.Quote]
	Passthrough Quoter
	TTL         time.Duration
}

func NewCache(passthru Quoter, capacity uint) (*Cache, error) {

	return &Cache{
		underlyer:   collection.NewLRUCache[normalize.Symbol, normalize.Quote](capacity),
		Passthrough: passthru,
	}, nil
}

func (c Cache) QuoteSymbol(ctx context.Context, symbol normalize.Symbol) (normalize.Quote, error) {
	if c.TTL == time.Duration(0) {
		c.TTL = 24 * time.Hour
	}

	staleAt := time.Now().Add(-c.TTL)

	if val, ok := c.underlyer.Get(symbol); ok && val.LastRefreshed.After(staleAt) {
		log.Println("returning cached value for ", symbol)
		return val, nil
	}

	result, err := c.Passthrough.QuoteSymbol(ctx, symbol)
	if err != nil {
		return normalize.Quote{}, err
	}
	c.underlyer.Put(symbol, result)
	return result, nil
}
