package colonio

var (
	ErrUndefined           = newErr(0, "")
	ErrSystemError         = newErr(1, "")
	ErrOffline             = newErr(2, "")
	ErrIncorrectDataFormat = newErr(3, "")
	ErrConflictWithSetting = newErr(4, "")
	ErrNotExistKey         = newErr(5, "")
	ErrExistKey            = newErr(6, "")
	ErrChangedProposer     = newErr(7, "")
	ErrCollisionLate       = newErr(8, "")
	ErrNoOneRecv           = newErr(9, "")
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
