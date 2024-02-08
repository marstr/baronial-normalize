package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	normalize "github.com/marstr/baronial-normalize"
)

type Client struct {
	Address string
}

func (c Client) QuoteSymbol(ctx context.Context, symbol normalize.Symbol) (normalize.Quote, error) {
	quotePath := "http://" + c.Address + "/api/v0/quote"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, quotePath, nil)
	if err != nil {
		return normalize.Quote{}, err
	}

	q := req.URL.Query()
	q.Set("symbol", string(symbol))
	req.URL.RawQuery = q.Encode()

	log.Printf("querying upstream %q for %s\n", c.Address, symbol)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return normalize.Quote{}, err
	}

	if resp.StatusCode == http.StatusOK {
		var retval normalize.Quote
		dec := json.NewDecoder(resp.Body)
		err = dec.Decode(&retval)
		return retval, err
	}

	return normalize.Quote{}, fmt.Errorf("unexpected upstream status code %d", resp.StatusCode)
}
