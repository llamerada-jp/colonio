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

#include "seed_link_websocket_wasm.hpp"

#include <emscripten.h>

#include <cassert>

extern "C" {
typedef unsigned long COLONIO_PTR_T;

extern void seed_link_ws_connect(COLONIO_PTR_T this_ptr, COLONIO_PTR_T url_ptr, int url_siz);
extern void seed_link_ws_disconnect(COLONIO_PTR_T this_ptr);
extern void seed_link_ws_finalize(COLONIO_PTR_T this_ptr);
extern void seed_link_ws_send(COLONIO_PTR_T this_ptr, COLONIO_PTR_T data_ptr, int data_siz);

EMSCRIPTEN_KEEPALIVE void seed_link_ws_on_connect(COLONIO_PTR_T this_ptr);
EMSCRIPTEN_KEEPALIVE void seed_link_ws_on_disconnect(COLONIO_PTR_T this_ptr);
EMSCRIPTEN_KEEPALIVE void seed_link_ws_on_error(COLONIO_PTR_T this_ptr, COLONIO_PTR_T msg_ptr, int msg_siz);
EMSCRIPTEN_KEEPALIVE void seed_link_ws_on_recv(COLONIO_PTR_T this_ptr, COLONIO_PTR_T data_ptr, int data_siz);
}

void seed_link_ws_on_connect(COLONIO_PTR_T this_ptr) {
  colonio::SeedLinkWebsocketWasm& THIS = *reinterpret_cast<colonio::SeedLinkWebsocketWasm*>(this_ptr);
  assert(THIS.debug_ptr == &THIS);

  THIS.delegate.seed_link_on_connect(THIS);
}

void seed_link_ws_on_disconnect(COLONIO_PTR_T this_ptr) {
  colonio::SeedLinkWebsocketWasm& THIS = *reinterpret_cast<colonio::SeedLinkWebsocketWasm*>(this_ptr);
  assert(THIS.debug_ptr == &THIS);

  THIS.delegate.seed_link_on_disconnect(THIS);
}

void seed_link_ws_on_error(COLONIO_PTR_T this_ptr, COLONIO_PTR_T msg_ptr, int msg_siz) {
  colonio::SeedLinkWebsocketWasm& THIS = *reinterpret_cast<colonio::SeedLinkWebsocketWasm*>(this_ptr);
  assert(THIS.debug_ptr == &THIS);

  std::string err = std::string(reinterpret_cast<const char*>(msg_ptr), msg_siz);
  THIS.delegate.seed_link_on_error(THIS, err);
}

void seed_link_ws_on_recv(COLONIO_PTR_T this_ptr, COLONIO_PTR_T data_ptr, int data_siz) {
  colonio::SeedLinkWebsocketWasm& THIS = *reinterpret_cast<colonio::SeedLinkWebsocketWasm*>(this_ptr);
  assert(THIS.debug_ptr == &THIS);

  THIS.delegate.seed_link_on_recv(THIS, std::string(reinterpret_cast<const char*>(data_ptr), data_siz));
}

namespace colonio {
SeedLinkWebsocketWasm::SeedLinkWebsocketWasm(SeedLinkParam& param) : SeedLink(param) {
#ifndef NDEBUG
  debug_ptr = this;
#endif
}

SeedLinkWebsocketWasm::~SeedLinkWebsocketWasm() {
  seed_link_ws_finalize(reinterpret_cast<COLONIO_PTR_T>(this));

#ifndef NDEBUG
  debug_ptr = nullptr;
#endif
}

void SeedLinkWebsocketWasm::connect(const std::string& url) {
  seed_link_ws_connect(reinterpret_cast<COLONIO_PTR_T>(this), reinterpret_cast<COLONIO_PTR_T>(url.c_str()), url.size());
}

void SeedLinkWebsocketWasm::disconnect() {
  seed_link_ws_disconnect(reinterpret_cast<COLONIO_PTR_T>(this));
}

void SeedLinkWebsocketWasm::send(const std::string& data) {
  seed_link_ws_send(reinterpret_cast<COLONIO_PTR_T>(this), reinterpret_cast<COLONIO_PTR_T>(data.c_str()), data.size());
}
}  // namespace colonio
