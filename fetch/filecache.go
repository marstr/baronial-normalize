package fetch

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path"
	"strings"
	"time"

	normalize "github.com/marstr/baronial-normalize"
)

type FileCache struct {
	Dir         string
	Passthrough Quoter
	TTL         time.Duration
}

func NewFileCache(passthru Quoter, dir string) (*FileCache, error) {
	return &FileCache{
		Dir:         dir,
		Passthrough: passthru,
	}, nil
}

func (fc FileCache) QuoteSymbol(ctx context.Context, symbol normalize.Symbol) (normalize.Quote, error) {
	if fc.TTL == time.Duration(0) {
		fc.TTL = 24 * time.Hour
	}

	staleAt := time.Now().Add(-fc.TTL)

	fileLoc := fc.getFileLocation(symbol)
	if file, err := os.Open(fileLoc); err == nil {
		var prev normalize.Quote
		dec := json.NewDecoder(file)
		err = dec.Decode(&prev)
		file.Close()
		if err != nil {
			return normalize.Quote{}, err
		}

		if prev.LastRefreshed.After(staleAt) {
			log.Println("returning file cached value for ", symbol)
			return prev, nil
		}
	}

	updated, err := fc.Passthrough.QuoteSymbol(ctx, symbol)
	if err != nil {
		return normalize.Quote{}, err
	}

	err = fc.WriteQuote(ctx, updated)

	return updated, err
}

func (fc FileCache) WriteQuote(ctx context.Context, quote normalize.Quote) error {
	fileLoc := fc.getFileLocation(quote.Symbol)
	var err error
	var file *os.File
	if file, err = os.Create(fileLoc); err == nil {
		defer file.Close()
		enc := json.NewEncoder(file)
		err = enc.Encode(quote)
	}
	return err
}

func (fc FileCache) getFileLocation(symbol normalize.Symbol) string {
	treated := strings.ToLower(string(symbol))
	treated = treated + ".json"
	return path.Join(fc.Dir, treated)
}
