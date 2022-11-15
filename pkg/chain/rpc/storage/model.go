package storage

import (
	"github.com/centrifuge/go-substrate-rpc-client/v4/scale"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

type MerchantInfo struct {
	DiskTotal types.U64
	DiskFree  types.U64
	TaskCount types.U8
}

type StateEnum struct {
	Storing bool
}

func (m *StateEnum) Decode(decoder scale.Decoder) error {
	b, err := decoder.ReadOneByte()

	if err != nil {
		return err
	}

	if b == 0 {
		m.Storing = true
	} else if b == 1 {
		m.Storing = false
	}

	if err != nil {
		return err
	}

	return nil
}

func (m StateEnum) Encode(encoder scale.Encoder) error {
	var err1, err2 error
	if m.Storing {
		err1 = encoder.PushByte(0)
	} else if !m.Storing {
		err1 = encoder.PushByte(1)
	}

	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}

	return nil
}

type StoreInfo struct {
	User  types.AccountID
	State StateEnum
}

type OrderInfo struct {
	CreateAt    types.BlockNumber
	FileHash    types.AccountID
	FileSize    types.U64
	FileCopy    types.U64
	ExpiredAt   types.BlockNumber
	StorageInfo []struct{ StoreInfo }
	Price       types.U64
}
