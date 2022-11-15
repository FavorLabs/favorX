package storage

import (
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

type MerchantInfo struct {
	DiskTotal types.U64
	DiskFree  types.U64
	TaskCount types.U8
}

type StoreInfo struct {
	User types.AccountID
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
