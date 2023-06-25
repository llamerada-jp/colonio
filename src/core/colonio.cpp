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
#include "colonio/colonio.hpp"

#ifdef __clang__
#  include <picojson.h>
#else
#  pragma GCC diagnostic push
#  pragma GCC diagnostic ignored "-Wmaybe-uninitialized"
#  include <picojson.h>
#  pragma GCC diagnostic pop
#endif

#include "colonio_impl.hpp"

namespace colonio {

void default_logger_func(Colonio&, const std::string& json) {
  picojson::value v;
  std::string err = picojson::parse(v, json);
  if (!err.empty()) {
    std::cerr << err << std::endl;
    return;
  }

  picojson::object json_obj = v.get<picojson::object>();
  std::string level         = json_obj.at("level").get<std::string>();

  if (level == "info") {
    std::cout << json << std::endl;
  } else {
    std::cerr << json << std::endl;
  }
}

ColonioConfig::ColonioConfig() :
    disable_callback_thread(false),
    disable_seed_verification(false),
    max_user_threads(1),
    seed_session_timeout_ms(30 * 1000),
    logger_func(default_logger_func) {
}

Colonio* Colonio::new_instance(ColonioConfig& config) {
  return new ColonioImpl(config);
}

Colonio::Colonio() {
}

Colonio::~Colonio() {
}

MessagingResponseWriter::~MessagingResponseWriter() {
}
}  // namespace colonio
