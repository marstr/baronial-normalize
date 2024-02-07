package alphavantage

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	normalize "github.com/marstr/baronial-normalize"
)

const (
	AlphaVantageBaseUri = "https://www.alphavantage.co/query"
)

type Client struct {
	ApiKey string
}

type ServerError Response

func (se ServerError) Error() string {
	return se.Information
}

type Response struct {
	GlobalQuote GlobalQuoteResponse `json:"Global Quote"`
	Information string              `json:"Information,omitempty"`
}

type GlobalQuoteResponse struct {
	Symbol           string  `json:"01. symbol"`
	Open             float64 `json:"02. open,string"`
	High             float64 `json:"03. high,string"`
	Low              float64 `json:"04. low,string"`
	Price            float64 `json:"05. price,string"`
	Volume           uint    `json:"06. volume,string"`
	LatestTradingDay string  `json:"07. latest trading day"`
	PreviousClose    float64 `json:"08. previous close,string"`
	Change           float64 `json:"09. change,string"`
	ChangePercent    string  `json:"10. change percent"`
}

func (avc Client) RawQuoteSymbol(ctx context.Context, symbol normalize.Symbol) (GlobalQuoteResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.alphavantage.co/query", nil)
	if err != nil {
		return GlobalQuoteResponse{}, err
	}

	q := req.URL.Query()
	q.Set("function", "GLOBAL_QUOTE")
	q.Set("symbol", string(symbol))
	q.Set("apikey", avc.ApiKey)
	req.URL.RawQuery = q.Encode()

	log.Println("querying Alpha Vantage API for ", symbol)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return GlobalQuoteResponse{}, err
	}

	rawResponse, err := io.ReadAll(resp.Body)
	if err != nil {
		return GlobalQuoteResponse{}, err
	}

	var unmarshaled Response
	err = json.Unmarshal(rawResponse, &unmarshaled)
	if err != nil {
		return GlobalQuoteResponse{}, err
	}

	if unmarshaled.Information != "" {
		return GlobalQuoteResponse{}, ServerError(unmarshaled)
	}

	return unmarshaled.GlobalQuote, nil
}

func (avc Client) QuoteSymbol(ctx context.Context, symbol normalize.Symbol) (normalize.Quote, error) {

	wrapped, err := avc.RawQuoteSymbol(ctx, symbol)
	if err != nil {
		return normalize.Quote{}, err
	}

	refreshedAt, err := time.Parse(time.DateOnly, wrapped.LatestTradingDay)
	if err != nil {
		return normalize.Quote{}, err
	}

	return normalize.Quote{
		Symbol:        symbol,
		FromSymbol:    "usd",
		Price:         wrapped.Price,
		LastRefreshed: refreshedAt,
	}, nil
}
