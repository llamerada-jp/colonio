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
#pragma once

#include <websocketpp/client.hpp>
#include <websocketpp/config/asio_client.hpp>

#include "seed_link.hpp"

namespace colonio {
class SeedLinkWebsocketNative : public SeedLinkBase {
 public:
  SeedLinkWebsocketNative(SeedLinkDelegate& delegate_, Context& context_);
  virtual ~SeedLinkWebsocketNative();

  // SeedlinkBase
  void connect(const std::string& url) override;
  void disconnect() override;
  void send(const std::string& data) override;

 private:
  typedef websocketpp::client<websocketpp::config::asio_client> WSClient;
  typedef websocketpp::config::asio_client::message_type::ptr message_ptr;
  WSClient client;
  WSClient::connection_ptr con;
  websocketpp::lib::shared_ptr<websocketpp::lib::thread> m_thread;
};

typedef SeedLinkWebsocketNative SeedLink;
}  // namespace colonio
