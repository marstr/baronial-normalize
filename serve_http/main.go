package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	normalize "github.com/marstr/baronial-normalize"
	"github.com/marstr/baronial-normalize/fetch"
	"github.com/marstr/baronial-normalize/fetch/alphavantage"
	"github.com/marstr/baronial-normalize/fetch/upstream"
	"github.com/spf13/viper"
)

const defaultPort = 4754
const portKey = "PORT"

var httpConfig *viper.Viper = viper.New()

const alphavantageApiKeyKey = "AVKEY"

const upstreamKey = "UPSTREAM"

var cache *fetch.Cache

func main() {
	httpConfig.GetUint(portKey)
	http.HandleFunc("/api/v0/quote", getMethodEnforcement(quotehandlers))

	var quoteSrc fetch.Quoter

	if httpConfig.IsSet(upstreamKey) {
		upstreamAddr := httpConfig.GetString(upstreamKey)
		quoteSrc = upstream.Client{Address: upstreamAddr}
		log.Printf("Using upstream %s as source of quotes\n", upstreamAddr)
	}

	if httpConfig.IsSet(alphavantageApiKeyKey) {
		quoteSrc = alphavantage.Client{ApiKey: httpConfig.GetString(alphavantageApiKeyKey)}
		log.Println("Using Alpha Vantage as source of quotes")
	}

	if quoteSrc == nil {
		log.Fatal("No source of quotes configured.")
	}

	var err error
	cache, err = fetch.NewCache(quoteSrc, 100)
	cache.TTL = 72 * time.Hour
	if err != nil {
		log.Fatal(err)
	}

	address := fmt.Sprintf(":%d", httpConfig.GetUint(portKey))
	log.Println("Baronial Normalize API listening at: ", address)
	log.Fatal(http.ListenAndServe(address, nil))
}

var quotehandlers = map[string]http.HandlerFunc{
	http.MethodGet: GetQuoteV1,
	http.MethodPut: SetQuoteV1,
}

func GetQuoteV1(resp http.ResponseWriter, req *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp.Header().Add("Content-Type", "application/json; charset=utf-8")

	requestedSymbol := req.URL.Query().Get("symbol")
	if requestedSymbol == "" {
		resp.WriteHeader(http.StatusBadRequest)
		c := struct {
			Error string `json:"error"`
		}{`missing required query parameter, "symbol"`}
		errWriter := json.NewEncoder(resp)
		err := errWriter.Encode(c)
		if err != nil {
			log.Println("Error!", err)
		}
		return
	}

	log.Println("Get Quote: ", requestedSymbol)
	price, err := cache.QuoteSymbol(ctx, normalize.Symbol(requestedSymbol))
	if err != nil {
		resp.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(resp, err)
		return
	}

	resp.WriteHeader(http.StatusOK)
	out := json.NewEncoder(resp)
	out.Encode(price)
}

func SetQuoteV1(resp http.ResponseWriter, req *http.Request) {

}

func init() {
	httpConfig.AddConfigPath("http")
	httpConfig.SetEnvPrefix("BNHTTP")
	httpConfig.SetDefault(portKey, defaultPort)
	httpConfig.AutomaticEnv()
}

func getMethodEnforcement(handlers map[string]http.HandlerFunc) http.HandlerFunc {
	var generateQuoteHandlersError sync.Once
	var methodListPlain string
	var methodListJson json.RawMessage

	return func(resp http.ResponseWriter, req *http.Request) {
		handler, ok := handlers[req.Method]
		if ok {
			handler(resp, req)
		} else {
			generateQuoteHandlersError.Do(func() {
				keys := make([]string, 0, len(quotehandlers))
				for k := range quotehandlers {
					keys = append(keys, k)
				}
				sort.Strings(keys)

				var listBuf bytes.Buffer

				for _, method := range keys {
					_, err := fmt.Fprintf(&listBuf, "%s, ", method)
					if err != nil {
						log.Fatal(err)
					}
				}
				listBuf.Truncate(listBuf.Len() - 2)
				methodListPlain = listBuf.String()

				var jsonBuf bytes.Buffer
				acceptedListEncoder := json.NewEncoder(&jsonBuf)
				err := acceptedListEncoder.Encode(keys)
				if err != nil {
					log.Fatal(err)
				}
				methodListJson = json.RawMessage(jsonBuf.Bytes())
			})

			resp.Header().Add("Allow", methodListPlain)
			resp.Header().Add("Content-Type", "application/json; charset=utf-8")
			resp.WriteHeader(http.StatusMethodNotAllowed)

			c := struct {
				Error    string           `json:"error"`
				Accepted *json.RawMessage `json:"accepted"`
			}{fmt.Sprintf("%q is not an accepted HTTP Method for this operation.", req.Method), &methodListJson}
			respBodyWriter := json.NewEncoder(resp)
			err := respBodyWriter.Encode(c)
			if err != nil {
				log.Println("Error: ", err)
			}
		}
	}
}
