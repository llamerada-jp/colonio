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
#pragma once

#include <functional>
#include <string>

namespace colonio {
class Logger;
class Scheduler;
class SeedLink;

struct SeedLinkParam {
  Logger& logger;
  std::string url;
  bool disable_verification;

  SeedLinkParam(Logger& l, const std::string& u, bool v);
};

class SeedLink {
 public:
  static SeedLink* new_instance(SeedLinkParam& param);

  virtual ~SeedLink();

  virtual void post(
      const std::string& path, const std::string& data, std::function<void(int, const std::string&)>&& cb_response) = 0;

 protected:
  Logger& logger;
  const std::string URL;
  const bool DISABLE_VERIFICATION;

  SeedLink(SeedLinkParam& param);
};
}  // namespace colonio
