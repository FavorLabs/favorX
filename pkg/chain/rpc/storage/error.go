package storage

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types/codec"
)

type Error struct {
	err error
}

var (
	DiskMustBeBetween1_4TB    = errors.New("DiskMustBeBetween1_4TB")
	MerchantHasLimit          = errors.New("MerchantHasLimit")
	MerchantDuplicate         = errors.New("MerchantDuplicate")
	NotEnoughMerchants        = errors.New("NotEnoughMerchants")
	MerchantBusy              = errors.New("MerchantBusy")
	MerchantDiskLessThan100MB = errors.New("MerchantDiskLessThan100MB")
	InsufficientBalance       = errors.New("InsufficientBalance")
	MinStorageTimesNotMeet    = errors.New("MinStorageTimesNotMeet")
	MinStorageFileCopyNotMeet = errors.New("MinStorageFileCopyNotMeet")
	OrderNotFound             = errors.New("OrderNotFound")
	OrderDuplicate            = errors.New("OrderDuplicate")
)

func NewError(moduleError types.ModuleError) *Error {
	b, e := codec.Encode(moduleError.Error)
	if e != nil {
		return &Error{err: e}
	}
	b = append(b, []byte{0, 0, 0, 0}...)
	var index int64
	e = binary.Read(bytes.NewBuffer(b), binary.LittleEndian, &index)
	if e != nil {
		return &Error{err: e}
	}
	var err = base.NotMatchModelError
	switch index {
	case 0:
		err = DiskMustBeBetween1_4TB
	case 1:
		err = MerchantHasLimit
	case 2:
		err = MerchantDuplicate
	case 3:
		err = NotEnoughMerchants
	case 4:
		err = MerchantBusy
	case 5:
		err = MerchantDiskLessThan100MB
	case 6:
		err = InsufficientBalance
	case 7:
		err = MinStorageTimesNotMeet
	case 8:
		err = MinStorageFileCopyNotMeet
	case 9:
		err = OrderNotFound
	case 10:
		err = OrderDuplicate
	}
	return &Error{err: err}
}

func (e *Error) Error() string {
	return e.err.Error()
}
