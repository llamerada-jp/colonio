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

#include <string>

#include "colonio/colonio.hpp"
#include "utils.hpp"

namespace colonio {
Error::Error(bool fatal_, ErrorCode code_, const std::string& message_, int line_, const std::string& file_) :
    fatal(fatal_), code(code_), message(message_), line(line_), file(Utils::file_basename(file_)) {
}

const char* Error::what() const noexcept {
  return message.c_str();
}
}  // namespace colonio
