/*
 * Copyright 2017-2019 Yuji Ito <llamerada.jp@gmail.com>
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

#include <cassert>
#include <colonio/colonio.hpp>

namespace colonio_helper {
class ColonioLibuv : public colonio::Colonio {
 public:
  uv_loop_t* loop;
  uv_async_t async;
  uv_timer_t timer;
  bool is_local;

  static void on_timer(uv_timer_t* handler) {
    ColonioLibuv& THIS = *reinterpret_cast<ColonioLibuv*>(handler->data);
    THIS.invoke_colonio();
  }

  static void on_async(uv_async_t* handler) {
    ColonioLibuv& THIS = *reinterpret_cast<ColonioLibuv*>(handler->data);
    THIS.invoke_colonio();
  }

  ColonioLibuv(uv_loop_t* loop_) : loop(loop_) {
    if (loop == nullptr) {
      loop     = uv_default_loop();
      is_local = true;
    } else {
      is_local = false;
    }

    // async
    uv_async_init(loop, &async, on_async);
    async.data = this;

    // timer
    uv_timer_init(loop, &timer);
    timer.data = this;
    uv_timer_start(&timer, on_timer, 1000, 1000);
  }

  virtual ~ColonioLibuv() {
    uv_timer_stop(&timer);

    if (is_local) {
      uv_loop_close(loop);
      loop = nullptr;
    }
  }

  void run(uv_run_mode mode = UV_RUN_DEFAULT) {
    uv_run(loop, mode);
  }

 protected:
  void on_require_invoke(unsigned int msec) override {
    uv_async_send(&async);
  }

  void invoke_colonio() {
    uv_timer_stop(&timer);
    unsigned int next_msec = invoke();

    uv_timer_start(&timer, on_timer, next_msec, 100);
  }
};
};  // namespace colonio_helper
