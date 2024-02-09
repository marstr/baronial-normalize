package normalize

import "fmt"

type Symbol string

type BadSymbol Symbol

func (bs BadSymbol) Error() string {
	return fmt.Sprintf("no result for symbol %q", string(bs))
}
