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

#ifndef EMSCRIPTEN
#  error For WebAssembly.
#endif

#include "http_accessor.hpp"

namespace colonio {
class HttpAccessorWasm : public HttpAccessorBase {
 public:
#ifndef NDEBUG
  HttpAccessorWasm* debug_ptr;
#endif

  HttpAccessorWasm();
  virtual ~HttpAccessorWasm();

  void post(const std::string& url, const std::string& payload,
            const std::string& content_type, HttpAccessorDelegate* delegate_) override;
};

typedef HttpAccessorWasm HttpAccessor;
}  // namespace colonio
