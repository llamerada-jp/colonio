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

import (
	"fmt"
	"log/slog"

	"go.etcd.io/raft/v3"
)

type slogWrapper struct {
	slogger *slog.Logger
}

var _ raft.Logger = (*slogWrapper)(nil)

func newSlogWrapper(slogger *slog.Logger) *slogWrapper {
	return &slogWrapper{
		slogger: slogger,
	}
}

func (s *slogWrapper) Debug(v ...interface{}) {
	s.slogger.Debug("etcd-raft", slog.Any("msg", v))
}

func (s *slogWrapper) Debugf(format string, v ...interface{}) {
	s.slogger.Debug("etcd-raft", slog.String("msg", fmt.Sprintf(format, v...)))
}

func (s *slogWrapper) Error(v ...interface{}) {
	s.slogger.Error("etcd-raft", slog.Any("msg", v))
}

func (s *slogWrapper) Errorf(format string, v ...interface{}) {
	s.slogger.Error("etcd-raft", slog.String("msg", fmt.Sprintf(format, v...)))
}

func (s *slogWrapper) Info(v ...interface{}) {
	s.slogger.Info("etcd-raft", slog.Any("msg", v))
}

func (s *slogWrapper) Infof(format string, v ...interface{}) {
	s.slogger.Info("etcd-raft", slog.String("msg", fmt.Sprintf(format, v...)))
}

func (s *slogWrapper) Warning(v ...interface{}) {
	s.slogger.Warn("etcd-raft", slog.Any("msg", v))
}

func (s *slogWrapper) Warningf(format string, v ...interface{}) {
	s.slogger.Warn("etcd-raft", slog.String("msg", fmt.Sprintf(format, v...)))
}

func (s *slogWrapper) Fatal(v ...interface{}) {
	s.slogger.Error("etcd-raft", slog.Any("msg", v))
	panic("etcd-raft fatal occurred")
}

func (s *slogWrapper) Fatalf(format string, v ...interface{}) {
	s.slogger.Error("etcd-raft", slog.String("msg", fmt.Sprintf(format, v...)))
	panic("etcd-raft fatal occurred")
}

func (s *slogWrapper) Panic(v ...interface{}) {
	s.slogger.Error("etcd-raft", slog.Any("msg", v))
	panic("etcd-raft panic occurred")
}

func (s *slogWrapper) Panicf(format string, v ...interface{}) {
	s.slogger.Error("etcd-raft", slog.String("msg", fmt.Sprintf(format, v...)))
	panic("etcd-raft panic occurred")
}
