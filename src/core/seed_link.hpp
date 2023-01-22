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

#include <string>

namespace colonio {
class Logger;
class Scheduler;
class SeedLink;
class SeedLinkDelegate;

struct SeedLinkParam {
  SeedLinkDelegate& delegate;
  Logger& logger;

  SeedLinkParam(SeedLinkDelegate& delegate_, Logger& logger_);
};

class SeedLinkDelegate {
 public:
  virtual ~SeedLinkDelegate();
  virtual void seed_link_on_connect(SeedLink& link)                           = 0;
  virtual void seed_link_on_disconnect(SeedLink& link)                        = 0;
  virtual void seed_link_on_error(SeedLink& link, const std::string& message) = 0;
  virtual void seed_link_on_recv(SeedLink& link, const std::string& data)     = 0;
};

class SeedLink {
 public:
  SeedLinkDelegate& delegate;

  static SeedLink* new_instance(SeedLinkParam& param);

  virtual ~SeedLink();

  virtual void connect(const std::string& url) = 0;
  virtual void disconnect()                    = 0;
  virtual void send(const std::string& data)   = 0;

 protected:
  Logger& logger;

  SeedLink(SeedLinkParam& param);
};
}  // namespace colonio
