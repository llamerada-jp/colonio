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
#include <cassert>

#include "webrtc_link.hpp"

namespace colonio {
/**
 * Simple destructor for vtable.
 */
WebrtcLinkDelegate::~WebrtcLinkDelegate() {
}

WebrtcLinkBase::InitData::InitData() :
    is_by_seed(false),
    is_changing_ice(false),
    is_prime(false),
    has_delete_func(false) {
}

WebrtcLinkBase::InitData::~InitData() {
  if (has_delete_func) {
    on_delete_func(on_delete_v);
  }
}

void WebrtcLinkBase::InitData::hook_on_delete(std::function<void(void*)> func, void* v) {
  assert(has_delete_func == false);

  has_delete_func   = true;
  on_delete_func    = func;
  on_delete_v       = v;
}

WebrtcLinkBase::WebrtcLinkBase(WebrtcLinkDelegate& delegate_, Context& context_, WebrtcContext& webrtc_context_) :
    delegate(delegate_),
    init_data(std::make_unique<InitData>()),
    context(context_),
    webrtc_context(webrtc_context_) {
}

WebrtcLinkBase::~WebrtcLinkBase() {
}
}  // namespace colonio
