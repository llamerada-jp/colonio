package colonio

// Error codes returned by failures to processing some commands.
var (
	ErrUndefined           = newErr(0, "") // ErrUndefined meaning undefined error is occurred.
	ErrSystemError         = newErr(1, "") // ErrSystemError meaning an error occurred in the API, which is used inside colonio.
	ErrOffline             = newErr(2, "") // ErrOffline meaning the node cannot perform processing because of offline.
	ErrIncorrectDataFormat = newErr(3, "") // ErrIncorrectDataFormat meaning incorrect data format detected.
	ErrConflictWithSetting = newErr(4, "") // ErrConflictWithSetting meaning The calling method or setting parameter was inconsistent with the configuration in the seed.
	ErrNotExistKey         = newErr(5, "") // ErrNotExistKey meaning tried to get a value for a key that doesn't exist.
	ErrExistKey            = newErr(6, "") // ErrExistKey meaning an error occurs when overwriting the value for an existing key.
	ErrChangedProposer     = newErr(7, "") // Developing
	ErrCollisionLate       = newErr(8, "") // Developing
	ErrNoOneRecv           = newErr(9, "") // ErrNoOneRecv meaning there was no node receiving the message.
)

type errImpl struct {
	code    uint32
	message string
}

func newErr(code uint32, message string) error {
	return &errImpl{
		code:    code,
		message: message,
	}
}

func (e *errImpl) Error() string {
	return e.message
}

func (e *errImpl) Is(target error) bool {
	t, ok := target.(*errImpl)
	if ok {
		return e.code == t.code
	}
	return false
}
