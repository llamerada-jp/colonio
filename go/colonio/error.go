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
	ErrUndefined                 = newErr(0, "", 0, "")
	ErrSystemIncorrectDataFormat = newErr(1, "", 0, "")
	ErrSystemConflictWithSetting = newErr(2, "", 0, "")
	ErrConnectionFailed          = newErr(3, "", 0, "")
	ErrConnectionOffline         = newErr(4, "", 0, "")
	ErrPacketNoOneRecv           = newErr(5, "", 0, "")
	ErrPacketTimeout             = newErr(6, "", 0, "")
	ErrMessagingHandlerNotFound  = newErr(7, "", 0, "")
	ErrKvsNotFound               = newErr(8, "", 0, "")
	ErrKvsProhibitOverwrite      = newErr(9, "", 0, "")
	ErrKvsCollision              = newErr(10, "", 0, "")
	ErrSpreadNoOneReceive        = newErr(11, "", 0, "")
)

type errImpl struct {
	code    uint32
	message string
	line    uint
	file    string
}

func newErr(code uint32, message string, line uint, file string) error {
	return &errImpl{
		code:    code,
		message: message,
		line:    line,
		file:    file,
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
