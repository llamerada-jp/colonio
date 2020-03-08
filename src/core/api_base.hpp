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

#include <memory>

#include "api.pb.h"
#include "colonio/exception.hpp"
#include "definition.hpp"

namespace colonio {
class APIBase;
class Context;

class APIDelegate {
 public:
  virtual ~APIDelegate();
  virtual void api_send_event(APIBase& api_base, std::unique_ptr<api::Event> event) = 0;
  virtual void api_send_reply(APIBase& api_base, std::unique_ptr<api::Reply> reply) = 0;
};

class APIBase {
 public:
  const APIChannel::Type channel;

  virtual ~APIBase();
  virtual void api_on_recv_call(const api::Call& call) = 0;

 protected:
  Context& context;

  APIBase(Context& context_, APIDelegate& delegate_, APIChannel::Type channel_);

  void api_event(std::unique_ptr<api::Event> event);
  void api_failure(uint32_t id, Exception::Code code, const std::string message);
  void api_reply(std::unique_ptr<api::Reply> reply);
  void api_success(uint32_t id);

 private:
  APIDelegate& delegate;
};
}  // namespace colonio
