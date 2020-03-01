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

#include "api_entry.hpp"

namespace colonio {
APIEntryDelegate::~APIEntryDelegate() {
}

APIEntry::APIEntry(Context& context_, APIEntryDelegate& delegate_, APIChannel::Type channel_) :
    channel(channel_),
    context(context_),
    delegate(delegate_) {
}

APIEntry::~APIEntry() {
}

void APIEntry::api_event(std::unique_ptr<api::Event> event) {
  assert(event->channel() == APIChannel::NONE);
  assert(event->param_case() != api::Event::ParamCase::PARAM_NOT_SET);

  event->set_channel(channel);
  delegate.api_entry_send_event(*this, std::move(event));
}

void APIEntry::api_failure(uint32_t id, const std::string message) {
  std::unique_ptr<api::Reply> reply = std::make_unique<api::Reply>();
  reply->set_id(id);

  api::Failure* param = reply->mutable_failure();
  param->set_message(message);

  delegate.api_entry_send_reply(*this, std::move(reply));
}

void APIEntry::api_reply(std::unique_ptr<api::Reply> reply) {
  assert(reply->id() != 0);
  assert(reply->param_case() != api::Reply::ParamCase::PARAM_NOT_SET);

  delegate.api_entry_send_reply(*this, std::move(reply));
}

void APIEntry::api_success(uint32_t id) {
  std::unique_ptr<api::Reply> reply = std::make_unique<api::Reply>();
  reply->set_id(id);

  reply->mutable_success();

  delegate.api_entry_send_reply(*this, std::move(reply));
}
}  // namespace colonio
