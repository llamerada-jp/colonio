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

#include <picojson.h>

#include <ctime>
#include <functional>
#include <string>

#include "definition.hpp"
#include "node_id.hpp"
#include "webrtc_context.hpp"

namespace colonio {
class Context;

#ifndef EMSCRIPTEN
class WebrtcLinkNative;
typedef WebrtcLinkNative WebrtcLink;
#else
class WebrtcLinkWasm;
typedef WebrtcLinkWasm WebrtcLink;
#endif

class WebrtcLinkDelegate {
 public:
  virtual ~WebrtcLinkDelegate();
  virtual void webrtc_link_on_change_status(WebrtcLink& link, LinkStatus::Type status)  = 0;
  virtual void webrtc_link_on_error(WebrtcLink& link)                                   = 0;
  virtual void webrtc_link_on_update_ice(WebrtcLink& link, const picojson::object& ice) = 0;
  virtual void webrtc_link_on_recv_data(WebrtcLink& link, const std::string& data)      = 0;
};

class WebrtcLinkBase {
 public:
  class InitData {
   public:
    bool is_by_seed;
    bool is_changing_ice;
    bool is_prime;
    int64_t start_time;
    picojson::array ice;

    InitData();
    virtual ~InitData();

    void hook_on_delete(std::function<void(void*)> func, void* v);

   private:
    bool has_delete_func;
    std::function<void(void*)> on_delete_func;
    void* on_delete_v;
  };
  /// Opposite peer's node-id.
  NodeID nid;
  /// Event handler.
  WebrtcLinkDelegate& delegate;
  ///
  std::unique_ptr<InitData> init_data;

  WebrtcLinkBase(WebrtcLinkDelegate& delegate_, Context& context_, WebrtcContext& webrtc_context);
  virtual ~WebrtcLinkBase();

  virtual void disconnect()                                                = 0;
  virtual void get_local_sdp(std::function<void(const std::string&)> func) = 0;
  virtual LinkStatus::Type get_status()                                    = 0;
  virtual bool send(const std::string& data)                               = 0;
  virtual void set_remote_sdp(const std::string& sdp)                      = 0;
  virtual void update_ice(const picojson::object& ice)                     = 0;

 protected:
  Context& context;
  WebrtcContext& webrtc_context;
};
}  // namespace colonio

#ifndef EMSCRIPTEN
#  include "webrtc_link_native.hpp"
#else
#  include "webrtc_link_wasm.hpp"
#endif
