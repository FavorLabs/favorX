package storage

import "github.com/centrifuge/go-substrate-rpc-client/v4/types"

type MerchantInfo struct {
	DiskTotal types.U64
	DiskFree  types.U64
	TaskCount types.U8
}
