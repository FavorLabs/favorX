package storagefiles

type Error struct {
	Message string `json:"message"`
	Code    int    `json:"code"`

	// unexported field
	internal bool
}

func (e Error) IsPublic() bool {
	return !e.internal
}

func (e Error) Error() string {
	return e.Message
}

func newError(code int, message string) Error {
	return Error{
		Message:  message,
		Code:     code,
		internal: false,
	}
}

func WrapError(code int, err error) Error {
	return Error{
		Message:  err.Error(),
		Code:     code,
		internal: false,
	}
}

var (
	ErrDiskFull = newError(1100, "disk space is full")
)
