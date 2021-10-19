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
#include <emscripten.h>

#include "http_accessor.hpp"

extern "C" {
typedef unsigned long COLONIO_PTR_T;

extern void http_accessor_initialize(COLONIO_PTR_T http_accessor);
extern void http_accessor_finalize(COLONIO_PTR_T http_accessor);
extern void http_accessor_post(
    COLONIO_PTR_T http_accessor, COLONIO_PTR_T delegate, COLONIO_PTR_T url, COLONIO_PTR_T content_type,
    COLONIO_PTR_T payload_ptr, int payload_siz);
EMSCRIPTEN_KEEPALIVE void http_accessor_on_response(
    COLONIO_PTR_T this_ptr, COLONIO_PTR_T delegate_ptr, int code_ptr, COLONIO_PTR_T content, int content_siz);
EMSCRIPTEN_KEEPALIVE void http_accessor_on_error(
    COLONIO_PTR_T this_ptr, COLONIO_PTR_T delegate_ptr, COLONIO_PTR_T message_ptr, int message_siz,
    COLONIO_PTR_T payload_ptr, COLONIO_PTR_T payload_siz);
}

void http_accessor_on_response(
    COLONIO_PTR_T this_ptr, COLONIO_PTR_T delegate_ptr, int code, COLONIO_PTR_T content_ptr, int content_siz) {
  colonio::HttpAccessorWasm& THIS         = *reinterpret_cast<colonio::HttpAccessorWasm*>(this_ptr);
  colonio::HttpAccessorDelegate& delegate = *reinterpret_cast<colonio::HttpAccessorDelegate*>(delegate_ptr);
  assert(THIS.debug_ptr == &THIS);
  std::string content(reinterpret_cast<char*>(content_ptr), content_siz);

  delegate.http_accessor_on_response(THIS, code, content);
}

void http_accessor_on_error(
    COLONIO_PTR_T this_ptr, COLONIO_PTR_T delegate_ptr, COLONIO_PTR_T message_ptr, int message_siz,
    COLONIO_PTR_T payload_ptr, COLONIO_PTR_T payload_siz) {
  colonio::HttpAccessorWasm& THIS         = *reinterpret_cast<colonio::HttpAccessorWasm*>(this_ptr);
  colonio::HttpAccessorDelegate& delegate = *reinterpret_cast<colonio::HttpAccessorDelegate*>(delegate_ptr);
  assert(THIS.debug_ptr == &THIS);
  std::string message(reinterpret_cast<char*>(message_ptr), message_siz);
  std::string payload(reinterpret_cast<char*>(payload_ptr), payload_siz);

  delegate.http_accessor_on_error(THIS, message, payload);
}

namespace colonio {
HttpAccessorWasm::HttpAccessorWasm() {
#ifndef NDEBUG
  debug_ptr = this;
#endif
  http_accessor_initialize(reinterpret_cast<COLONIO_PTR_T>(this));
}

HttpAccessorWasm::~HttpAccessorWasm() {
  http_accessor_finalize(reinterpret_cast<COLONIO_PTR_T>(this));
}

void HttpAccessorWasm::post(
    const std::string& url, const std::string& payload, const std::string& content_type,
    HttpAccessorDelegate* delegate) {
  assert(delegate != nullptr);

  http_accessor_post(
      reinterpret_cast<COLONIO_PTR_T>(this), reinterpret_cast<COLONIO_PTR_T>(delegate),
      reinterpret_cast<COLONIO_PTR_T>(url.c_str()), reinterpret_cast<COLONIO_PTR_T>(content_type.c_str()),
      reinterpret_cast<COLONIO_PTR_T>(payload.c_str()), payload.size());
}
}  // namespace colonio
