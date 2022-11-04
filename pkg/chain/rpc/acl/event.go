package acl

import "github.com/centrifuge/go-substrate-rpc-client/v4/types"

type EventRecords struct {
	types.EventRecords
	Acl_NicknameSet     []EventAclNicknameSet
	Acl_NicknameRemoved []EventAclNicknameRemoved
	Acl_UriResolved     []EventAclUriResolved
	Acl_UriUnresolved   []EventAclUriUnresolved
	Acl_Authorized      []EventAclAuthorized
	Acl_Revoked         []EventAclRevoked
	Acl_DelegateSet     []EventAclDelegateSet
	Acl_DelegateRemoved []EventAclDelegateRemoved
}

type EventAclNicknameSet struct {
	Phase    types.Phase
	Who      types.AccountID
	Nickname types.Bytes
	Topics   []types.Hash
}

type EventAclNicknameRemoved struct {
	Phase    types.Phase
	Who      types.AccountID
	Nickname types.Bytes
	Topics   []types.Hash
}

type EventAclUriResolved struct {
	Phase    types.Phase
	Who      types.AccountID
	Path     types.Bytes
	FileHash types.Hash
	Topics   []types.Hash
}

type EventAclUriUnresolved struct {
	Phase  types.Phase
	Who    types.AccountID
	Path   types.Bytes
	Topics []types.Hash
}

type EventAclAuthorized struct {
	Phase    types.Phase
	Who      types.AccountID
	Path     types.Bytes
	AuthType types.Bytes
	Target   []types.AccountID
	ExpireAt types.BlockNumber
	Topics   []types.Hash
}

type EventAclRevoked struct {
	Phase    types.Phase
	Who      types.AccountID
	Path     types.Bytes
	AuthType types.Bytes
	Target   []types.AccountID
	Topics   []types.Hash
}

type EventAclDelegateSet struct {
	Phase    types.Phase
	Who      types.AccountID
	Path     types.Bytes
	AuthType types.Bytes
	Target   []types.AccountID
	ExpireAt types.BlockNumber
	Topics   []types.Hash
}

type EventAclDelegateRemoved struct {
	Phase    types.Phase
	Who      types.AccountID
	Path     types.Bytes
	AuthType types.Bytes
	Target   []types.AccountID
	Topics   []types.Hash
}
