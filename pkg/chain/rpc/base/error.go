package base

import "errors"

var (
	TokenError         = errors.New("token error")
	ArithmeticError    = errors.New("arithmetic error")
	TransactionalError = errors.New("transactional error")
	NotMatchModelError = errors.New("not match model error")
	KeyEmptyError      = errors.New("is empty")
)
