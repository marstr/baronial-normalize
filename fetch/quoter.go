package fetch

import (
	"context"

	normalize "github.com/marstr/baronial-normalize"
)

type Quoter interface {
	QuoteSymbol(context.Context, normalize.Symbol) (normalize.Quote, error)
}
