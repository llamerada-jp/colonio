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

#include "api_bundler.hpp"

#include "api_base.hpp"
#include "utils.hpp"

namespace colonio {
void APIBundler::call(const api::Call& call) {
  assert(call.id() != 0);

  auto api_base = apis.find(call.channel());
  if (api_base != apis.end()) {
    api_base->second->api_on_recv_call(call);
  } else {
    colonio_fatal("Called incorrect API entry : %d", call.channel());
  }
}

void APIBundler::registrate(std::shared_ptr<APIBase> api_base) {
  assert(api_base->channel != APIChannel::NONE);
  assert(apis.find(api_base->channel) == apis.end());

  apis.insert(std::make_pair(api_base->channel, api_base));
}
}  // namespace colonio
