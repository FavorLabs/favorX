package resolver

import (
	"io"

	"github.com/FavorLabs/favorX/pkg/boson"
)

// Address is the boson address.
type Address = boson.Address

// Interface can resolve a URL into an associated Ethereum address.
type Interface interface {
	Resolve(url string) (Address, error)
	io.Closer
}
