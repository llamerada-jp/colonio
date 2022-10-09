package colonio

/*
 * Copyright 2017 Yuji Ito <llamerada.jp@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Error codes returned by failures to processing some commands.
var (
	ErrUndefined           = newErr(0, "")  // ErrUndefined meaning undefined error is occurred.
	ErrSystemError         = newErr(1, "")  // ErrSystemError meaning an error occurred in the API, which is used inside colonio.
	ErrConnectionFailed    = newErr(2, "")  // ErrConnectionFailed
	ErrOffline             = newErr(3, "")  // ErrOffline meaning the node cannot perform processing because of offline.
	ErrIncorrectDataFormat = newErr(4, "")  // ErrIncorrectDataFormat meaning incorrect data format detected.
	ErrConflictWithSetting = newErr(5, "")  // ErrConflictWithSetting meaning The calling method or setting parameter was inconsistent with the configuration in the seed.
	ErrNotExistKey         = newErr(6, "")  // ErrNotExistKey meaning tried to get a value for a key that doesn't exist.
	ErrExistKey            = newErr(7, "")  // ErrExistKey meaning an error occurs when overwriting the value for an existing key.
	ErrChangedProposer     = newErr(8, "")  // Developing
	ErrCollisionLate       = newErr(9, "")  // Developing
	ErrNoOneRecv           = newErr(10, "") // ErrNoOneRecv meaning there was no node receiving the message.
	ErrCallbackError       = newErr(11, "") // ErrCallbackError meaning that an error occurred while executing the callback function.
	ErrRRCUndefinedError   = newErr(12, "") // ErrRPCUndefinedError occurred in callback function of call_by_nid.
	ErrTimeout             = newErr(13, "") // ErrTimeout meaning an error occurs when timeout.
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
