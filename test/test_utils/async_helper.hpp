/*
 * Copyright 2017-2020 Yuji Ito <llamerada.jp@gmail.com>
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
#pragma once

#include <uv.h>

#include <sstream>
#include <string>

class AsyncHelper {
 public:
  std::stringstream marks;
  std::unique_ptr<uv_loop_t> loop;

  AsyncHelper() {
  }

  virtual ~AsyncHelper() {
    if (loop) {
      uv_loop_close(loop.get());
    }
  }

  uv_loop_t* get_libuv_instance() {
    if (!loop) {
      loop.reset(new uv_loop_t());
      uv_loop_init(loop.get());
    }

    return loop.get();
  }

  void mark(const std::string& m) {
    marks << m;
  }

  void run() {
    uv_run(get_libuv_instance(), UV_RUN_DEFAULT);
  }

  std::string get_route() {
    return marks.str();
  }
};
