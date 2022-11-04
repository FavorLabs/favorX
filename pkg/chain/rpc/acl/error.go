package acl

import (
	"errors"

	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

type Error struct {
	err error
}

var (
	NicknameDuplicate   = errors.New("NicknameDuplicate")
	NicknameInvalid     = errors.New("NicknameInvalid")
	NicknameNonexistent = errors.New("NicknameNonexistent")
	UriInvalid          = errors.New("UriInvalid")
	PathInvalid         = errors.New("PathInvalid")
	MissingTargetUser   = errors.New("MissingTargetUser")
	NoPermission        = errors.New("NoPermission")
	AccountIdInvalid    = errors.New("AccountIdInvalid")
)

func NewError(index types.U8) *Error {
	var err = base.NotMatchModelError
	switch int(index) {
	case 0:
		err = NicknameDuplicate
	case 1:
		err = NicknameInvalid
	case 2:
		err = NicknameNonexistent
	case 3:
		err = UriInvalid
	case 4:
		err = PathInvalid
	case 5:
		err = MissingTargetUser
	case 6:
		err = NoPermission
	case 7:
		err = AccountIdInvalid
	}
	return &Error{err: err}
}

func (e *Error) Error() string {
	return e.err.Error()
}
