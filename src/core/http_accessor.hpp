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

#include <functional>
#include <string>

namespace colonio {
class HttpAccessorBase;

class HttpAccessorDelegate {
 public:
  virtual ~HttpAccessorDelegate();
  virtual void http_accessor_on_response(HttpAccessorBase& http, int code, const std::string& content)              = 0;
  virtual void http_accessor_on_error(HttpAccessorBase& http, const std::string& message, const std::string& field) = 0;
};

class HttpAccessorBase {
 public:
  virtual ~HttpAccessorBase();
  virtual void post(
      const std::string& url, const std::string& payload, const std::string& content_type,
      HttpAccessorDelegate* delegate) = 0;
};
}  // namespace colonio

#ifndef EMSCRIPTEN
#  include "http_accessor_curl.hpp"
#else
#  include "http_accessor_wasm.hpp"
#endif
