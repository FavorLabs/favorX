package storage

import (
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

type OrderMatchSuccess func(mch types.AccountID)

type MerchantInfo struct {
	DiskTotal types.U64
	DiskFree  types.U64
	TaskCount types.U8
}

type OrderInfo struct {
	CreateAt  types.U32
	FileHash  types.AccountID
	FileSize  types.U64
	FileCopy  types.U64
	ExpiredAt types.U32
	Merchants []types.AccountID
	Price     types.U64
}
