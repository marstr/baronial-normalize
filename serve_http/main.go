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
	"github.com/spf13/viper"
)

const defaultPort = 4754
const portKey = "PORT"

const alphavantageApiKeyKey = "AVKEY"

var httpConfig *viper.Viper = viper.New()

var alphaVantageClient alphavantage.Client
var cache *fetch.Cache

func main() {
	httpConfig.GetUint(portKey)
	http.HandleFunc("/api/v0/quote", getMethodEnforcement(quotehandlers))

	if httpConfig.IsSet(alphavantageApiKeyKey) {
		alphaVantageClient = alphavantage.Client{ApiKey: httpConfig.GetString(alphavantageApiKeyKey)}
	} else {
		log.Fatal("No Alpha Vantage API Key Found")
	}

	var err error
	cache, err = fetch.NewCache(alphaVantageClient, 100)
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
	var quoteMethodListPlain string
	var quoteMethodListJson *json.RawMessage
	var methodResponseBody *bytes.Buffer

	return func(resp http.ResponseWriter, req *http.Request) {
		handler, ok := handlers[req.Method]
		if !ok {
			generateQuoteHandlersError.Do(func() {
				keys := make([]string, 0, len(quotehandlers))
				for k := range quotehandlers {
					keys = append(keys, k)
				}
				sort.Strings(keys)

				var listBuf *bytes.Buffer

				for _, method := range keys {
					_, err := fmt.Fprintf(listBuf, "%s, ", method)
					if err != nil {
						log.Fatal(err)
					}
				}
				listBuf.Truncate(listBuf.Len() - 2)
				quoteMethodListPlain = listBuf.String()

				acceptedListEncoder := json.NewEncoder(methodResponseBody)
				err := acceptedListEncoder.Encode(keys)
				if err != nil {
					log.Fatal(err)
				}
			})

			resp.WriteHeader(http.StatusMethodNotAllowed)
			resp.Header().Add("Allow", quoteMethodListPlain)

			c := struct {
				Error    string           `json:"error"`
				Accepted *json.RawMessage `json:"accepted"`
			}{fmt.Sprintf("%q is not an accepted HTTP Method for this operation, see accepted", req.Method), quoteMethodListJson}
			respBodyWriter := json.NewEncoder(resp)
			err := respBodyWriter.Encode(c)
			if err != nil {
				log.Println("Error: ", err)
			}
		}

		handler(resp, req)
	}
}
