package address

import (
	"errors"
	"fmt"
	"github.com/FavorLabs/favorX/pkg/bitvector"
)

var ErrInvalidNodeMode = errors.New("invalid node mode")

const (
	FullNode = iota
	BootNode
)

const bitVictorNodeModeLen = 1
const RelayPrefixHttp = "/group/http"

func NewModel() Model {
	bv, _ := bitvector.New(bitVictorNodeModeLen)
	return Model{Bv: bv}
}

func NewModelFromBytes(m []byte) (Model, error) {
	bv, err := bitvector.NewFromBytes(m, bitVictorNodeModeLen)
	if err != nil {
		return NewModel(), err
	}
	return Model{Bv: bv}, nil
}

type Model struct {
	Bv *bitvector.BitVector
}

func (m Model) SetMode(md int) Model {
	m.Bv.Set(md)
	return m
}

func (m Model) IsFull() bool {
	return m.Bv.Get(FullNode)
}

func (m Model) IsBootNode() bool {
	return m.Bv.Get(BootNode)
}

func (m Model) String() string {
	if m.IsBootNode() {
		return "boot-node"
	}
	if !m.IsFull() {
		return "light"
	}

	return "full"
}

// AddressInfo contains the information received from the handshake.
type AddressInfo struct {
	Address  *Address
	NodeMode Model
}

func (i *AddressInfo) LightString() string {
	return fmt.Sprintf(" (%s)", i.NodeMode.String())
}
