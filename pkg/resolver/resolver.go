package resolver

import (
	"io"

	"github.com/FavorLabs/favorX/pkg/boson"
)

// Address is the boson aurora address.
type Address = boson.Address

// Interface can resolve an URL into an associated Ethereum address.
type Interface interface {
	Resolve(url string) (Address, error)
	io.Closer
}
