package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	normalize "github.com/marstr/baronial-normalize"
	"github.com/marstr/baronial-normalize/fetch"
	"github.com/marstr/baronial-normalize/fetch/alphavantage"
	"github.com/marstr/baronial-normalize/fetch/upstream"
	"github.com/marstr/units/data"
	"github.com/spf13/viper"
)

const defaultPort = 4754
const portKey = "PORT"

var httpConfig *viper.Viper = viper.New()

const alphavantageApiKeyKey = "AVKEY"

const upstreamKey = "UPSTREAM"
const fileCacheLocKey = "CACHEPATH"

const memCacheLimitKey = "MEMCACHE_LIMIT"
const memCacheLimitDefault = 100

const cacheTtlKey = "CACHE_TTLHOURS"
const cacheTtlDefault = 72

var quoter fetch.Quoter
var fileCache *fetch.FileCache

func main() {
	httpConfig.GetUint(portKey)
	http.HandleFunc("/api/v0/quote", getMethodEnforcement(quotehandlers))

	if httpConfig.IsSet(upstreamKey) {
		upstreamAddr := httpConfig.GetString(upstreamKey)
		quoter = upstream.Client{Address: upstreamAddr}
		log.Printf("Using upstream %s as source of quotes\n", upstreamAddr)
	}

	if httpConfig.IsSet(alphavantageApiKeyKey) {
		quoter = alphavantage.Client{ApiKey: httpConfig.GetString(alphavantageApiKeyKey)}
		log.Println("Using Alpha Vantage as source of quotes")
	}

	if quoter == nil {
		log.Fatal("No source of quotes configured.")
	}

	cacheTtl := time.Hour * time.Duration(httpConfig.GetInt(cacheTtlKey))
	if httpConfig.IsSet(fileCacheLocKey) {
		cacheLoc := httpConfig.GetString(fileCacheLocKey)
		log.Println("Using filecache at ", cacheLoc)
		var err error
		fileCache, err = fetch.NewFileCache(quoter, cacheLoc)
		fileCache.TTL = cacheTtl
		if err != nil {
			log.Fatal(err)
		}
		quoter = fileCache
	}

	memCacheLimit := httpConfig.GetUint(memCacheLimitKey)
	log.Println("Using memcache with quote limit of ", memCacheLimit)
	memCache, err := fetch.NewMemCache(quoter, memCacheLimit)
	if err != nil {
		log.Fatal(err)
	}
	memCache.TTL = cacheTtl
	quoter = memCache

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
	price, err := quoter.QuoteSymbol(ctx, normalize.Symbol(requestedSymbol))
	if err != nil && errors.Is(err, normalize.BadSymbol(requestedSymbol)) {
		resp.WriteHeader(http.StatusNotFound)
		c := struct {
			Error  string `json:"error"`
			Symbol string `json:"symbol"`
		}{err.Error(), requestedSymbol}
		enc := json.NewEncoder(resp)
		enc.Encode(c)
		return
	} else if err != nil {
		resp.WriteHeader(http.StatusInternalServerError)
		c := struct {
			Error string `json:"error"`
		}{err.Error()}
		enc := json.NewEncoder(resp)
		enc.Encode(c)
		return
	}

	resp.WriteHeader(http.StatusOK)
	out := json.NewEncoder(resp)
	out.Encode(price)
}

func SetQuoteV1(resp http.ResponseWriter, req *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp.Header().Add("Content-Type", "application/json; charset=utf-8")

	var toSet normalize.Quote
	limitReader := io.LimitReader(req.Body, int64(10*data.Kilobyte))
	dec := json.NewDecoder(limitReader)
	err := dec.Decode(&toSet)
	if err != nil {
		resp.WriteHeader(http.StatusBadRequest)
		c := struct {
			Error   string `json:"error"`
			Wrapped string `json:"detail"`
		}{"could not read body of request as quote", err.Error()}
		enc := json.NewEncoder(resp)
		enc.Encode(c)
		return
	}

	if toSet.LastRefreshed.Equal(time.Time{}) {
		toSet.LastRefreshed = time.Now()
	}

	err = fileCache.WriteQuote(ctx, toSet)
	if err != nil {
		resp.WriteHeader(http.StatusInternalServerError)
		c := struct {
			Error   string `json:"error"`
			Wrapped string `json:"detail"`
		}{"failed to write error", err.Error()}
		enc := json.NewEncoder(resp)
		enc.Encode(c)
		return
	}

	resp.WriteHeader(http.StatusNoContent)
}

func init() {
	httpConfig.AddConfigPath("http")
	httpConfig.SetEnvPrefix("BNHTTP")
	httpConfig.SetDefault(portKey, defaultPort)
	httpConfig.SetDefault(memCacheLimitKey, memCacheLimitDefault)
	httpConfig.SetDefault(cacheTtlKey, cacheTtlDefault)
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
