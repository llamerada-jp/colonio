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

#ifdef __clang__
#  include <picojson.h>
#else
#  pragma GCC diagnostic push
#  pragma GCC diagnostic ignored "-Wmaybe-uninitialized"
#  include <picojson.h>
#  pragma GCC diagnostic pop
#endif

#include <functional>
#include <memory>
#include <string>

#include "definition.hpp"
#include "node_id.hpp"

namespace colonio {
class Logger;
class WebrtcContext;
class WebrtcLink;
class WebrtcLinkDelegate;

struct WebrtcLinkParam {
  WebrtcLinkDelegate& delegate;
  Logger& logger;
  WebrtcContext& context;

  WebrtcLinkParam(WebrtcLinkDelegate& delegate_, Logger& logger_, WebrtcContext& context_);
};

class WebrtcLinkDelegate {
 public:
  virtual ~WebrtcLinkDelegate();
  virtual void webrtc_link_on_change_dco_state(WebrtcLink& link, LinkState::Type state) = 0;
  virtual void webrtc_link_on_change_pco_state(WebrtcLink& link, LinkState::Type state) = 0;
  virtual void webrtc_link_on_error(WebrtcLink& link)                                   = 0;
  virtual void webrtc_link_on_update_ice(WebrtcLink& link, const picojson::object& ice) = 0;
  virtual void webrtc_link_on_recv_data(WebrtcLink& link, const std::string& data)      = 0;
};

class WebrtcLink {
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

    void hook_on_delete(std::function<void(void*)>&& func, void* v);

   private:
    bool has_delete_func;
    std::function<void(void*)> on_delete_func;
    void* on_delete_v;
  };

  /// Opposite peer's node-id.
  NodeID nid;
  /// Event handler.
  WebrtcLinkDelegate& delegate;
  LinkState::Type link_state;
  LinkState::Type dco_state;
  LinkState::Type pco_state;
  ///
  std::unique_ptr<InitData> init_data;

  static WebrtcLink* new_instance(WebrtcLinkParam& param, bool is_create_dc);

  virtual ~WebrtcLink();

  virtual void disconnect()                                                  = 0;
  virtual void get_local_sdp(std::function<void(const std::string&)>&& func) = 0;
  virtual LinkState::Type get_new_link_state()                               = 0;
  virtual bool send(const std::string& data)                                 = 0;
  virtual void set_remote_sdp(const std::string& sdp)                        = 0;
  virtual void update_ice(const picojson::object& ice)                       = 0;

 protected:
  Logger& logger;
  WebrtcContext& webrtc_context;

  WebrtcLink(WebrtcLinkParam& param);
};
}  // namespace colonio
