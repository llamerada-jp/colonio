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

#include "seed_link_wasm.hpp"

#include <emscripten.h>

#include <cassert>

extern "C" {
typedef unsigned long COLONIO_PTR_T;

extern void seed_link_post(
    COLONIO_PTR_T task_ptr, COLONIO_PTR_T url_ptr, int url_siz, COLONIO_PTR_T data_ptr, int data_siz);

EMSCRIPTEN_KEEPALIVE void seed_link_on_error(COLONIO_PTR_T task_ptr, COLONIO_PTR_T msg_ptr, int msg_siz);
EMSCRIPTEN_KEEPALIVE void seed_link_on_response(COLONIO_PTR_T task_ptr, int code, COLONIO_PTR_T data_ptr, int data_siz);
}

void seed_link_on_error(COLONIO_PTR_T task_ptr, COLONIO_PTR_T msg_ptr, int msg_siz) {
  colonio::SeedLinkWasm::Task& task = *reinterpret_cast<colonio::SeedLinkWasm::Task*>(task_ptr);

  if (!task.canceled) {
    std::string err = std::string(reinterpret_cast<const char*>(msg_ptr), msg_siz);
    task.cb(0, err);
  }
  colonio::SeedLinkWasm::tasks.erase(&task);
}

void seed_link_on_response(COLONIO_PTR_T task_ptr, int code, COLONIO_PTR_T data_ptr, int data_siz) {
  colonio::SeedLinkWasm::Task& task = *reinterpret_cast<colonio::SeedLinkWasm::Task*>(task_ptr);

  if (!task.canceled) {
    std::string data = std::string(reinterpret_cast<const char*>(data_ptr), data_siz);
    task.cb(code, data);
  }
  colonio::SeedLinkWasm::tasks.erase(&task);
}

namespace colonio {

std::set<SeedLinkWasm::Task*> SeedLinkWasm::tasks;

SeedLinkWasm::Task::Task(std::function<void(int, const std::string&)> c) : canceled(false), cb(c) {
}

SeedLinkWasm::SeedLinkWasm(SeedLinkParam& param) : SeedLink(param) {
}

SeedLinkWasm::~SeedLinkWasm() {
  for (auto& task : SeedLinkWasm::tasks) {
    task->canceled = true;
  }
}

void SeedLinkWasm::post(
    const std::string& path, const std::string& data, std::function<void(int, const std::string&)>&& cb_response) {
  Task* task      = new Task(cb_response);
  std::string url = URL + path;
  SeedLinkWasm::tasks.insert(task);
  seed_link_post(
      reinterpret_cast<COLONIO_PTR_T>(task), reinterpret_cast<COLONIO_PTR_T>(url.c_str()), url.size(),
      reinterpret_cast<COLONIO_PTR_T>(data.c_str()), data.size());
}
}  // namespace colonio
