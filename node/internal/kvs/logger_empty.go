/*
 * Copyright 2017- Yuji Ito <llamerada.jp@gmail.com>
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
package kvs

import "go.etcd.io/raft/v3"

type emptyLogger struct{}

var _ raft.Logger = (*emptyLogger)(nil)

func newEmptyLogger() *emptyLogger {
	return &emptyLogger{}
}

func (e *emptyLogger) Debug(v ...interface{})                   {}
func (e *emptyLogger) Debugf(format string, v ...interface{})   {}
func (e *emptyLogger) Error(v ...interface{})                   {}
func (e *emptyLogger) Errorf(format string, v ...interface{})   {}
func (e *emptyLogger) Info(v ...interface{})                    {}
func (e *emptyLogger) Infof(format string, v ...interface{})    {}
func (e *emptyLogger) Warning(v ...interface{})                 {}
func (e *emptyLogger) Warningf(format string, v ...interface{}) {}
func (e *emptyLogger) Fatal(v ...interface{})                   {}
func (e *emptyLogger) Fatalf(format string, v ...interface{})   {}
func (e *emptyLogger) Panic(v ...interface{})                   {}
func (e *emptyLogger) Panicf(format string, v ...interface{})   {}
