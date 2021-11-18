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
#include "webrtc_link.hpp"

#include <cassert>

#ifndef EMSCRIPTEN
#  include "webrtc_link_native.hpp"
#else
#  include "webrtc_link_wasm.hpp"
#endif

namespace colonio {
WebrtcLinkParam::WebrtcLinkParam(WebrtcLinkDelegate& delegate_, Logger& logger_, WebrtcContext& context_) :
    delegate(delegate_), logger(logger_), context(context_) {
}

/**
 * Simple destructor for vtable.
 */
WebrtcLinkDelegate::~WebrtcLinkDelegate() {
}

WebrtcLink::InitData::InitData() :
    is_by_seed(false),
    is_changing_ice(false),
    is_prime(false),
    start_time(0),
    has_delete_func(false),
    on_delete_v(nullptr) {
}

WebrtcLink::InitData::~InitData() {
  if (has_delete_func) {
    on_delete_func(on_delete_v);
  }
}

void WebrtcLink::InitData::hook_on_delete(std::function<void(void*)>&& func, void* v) {
  assert(has_delete_func == false);

  has_delete_func = true;
  on_delete_func  = func;
  on_delete_v     = v;
}

WebrtcLink* WebrtcLink::new_instance(WebrtcLinkParam& param, bool is_create_dc) {
#ifndef EMSCRIPTEN
  return new WebrtcLinkNative(param, is_create_dc);
#else
  return new WebrtcLinkWasm(param, is_create_dc);
#endif
}

WebrtcLink::WebrtcLink(WebrtcLinkParam& param) :
    delegate(param.delegate),
    link_state(LinkStatus::CONNECTING),
    dco_state(LinkStatus::CONNECTING),
    pco_state(LinkStatus::CONNECTING),
    init_data(std::make_unique<InitData>()),
    logger(param.logger),
    webrtc_context(param.context) {
}

WebrtcLink::~WebrtcLink() {
}
}  // namespace colonio
