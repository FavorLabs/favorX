package storage

import "github.com/centrifuge/go-substrate-rpc-client/v4/types"

type EventRecords struct {
	types.EventRecords
	Storage_MerchantRegister    []EventStorageMerchantRegister
	Storage_MerchantUnregister  []EventStorageMerchantUnregister
	Storage_OrderMatchSuccess   []EventStorageOrderMatchSuccess
	Storage_OrderStorageSuccess []EventStorageOrderStorageSuccess
	Storage_OrderTimeout        []EventStorageOrderTimeout
}

type EventStorageMerchantRegister struct {
	Phase     types.Phase
	AccountID types.AccountID
	DiskTotal types.U64
	Topics    []types.Hash
}

type EventStorageMerchantUnregister struct {
	Phase     types.Phase
	AccountID types.AccountID
	DiskTotal types.U64
	DiskFree  types.U64
	Reason    types.Bytes
	Topics    []types.Hash
}

type EventStorageOrderMatchSuccess struct {
	Phase    types.Phase
	Buyer    types.AccountID
	FileHash types.Hash
	Merchant types.AccountID
	Topics   []types.Hash
}

type EventStorageOrderTimeout struct {
	Phase  types.Phase
	Buyer  types.AccountID
	Cid    types.Hash
	Reason types.Bytes
	Topics []types.Hash
}

type EventStorageOrderStorageSuccess struct {
	Phase  types.Phase
	Buyer  types.AccountID
	Cid    types.Hash
	Topics []types.Hash
}
