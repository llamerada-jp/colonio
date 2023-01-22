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

#include "seed_link_websocket_native.hpp"

#include "logger.hpp"

namespace colonio {
SeedLinkWebsocketNative::SeedLinkWebsocketNative(SeedLinkParam& param) : SeedLink(param) {
  client.clear_access_channels(websocketpp::log::alevel::all);
  client.clear_error_channels(websocketpp::log::elevel::all);

  client.init_asio();
  client.start_perpetual();

  m_thread = websocketpp::lib::make_shared<websocketpp::lib::thread>(&WSClient::run, &client);
}

SeedLinkWebsocketNative::~SeedLinkWebsocketNative() {
  client.stop_perpetual();

  websocketpp::lib::error_code ec;
  if (con) {
    client.close(con->get_handle(), websocketpp::close::status::going_away, "", ec);
    if (ec) {
      log_debug("error on close websocket").map("message", ec.message());
    }
  }

  m_thread->join();
}

void SeedLinkWebsocketNative::connect(const std::string& url) {
  websocketpp::lib::error_code ec;

  con = client.get_connection(url, ec);

  if (ec) {
    this->delegate.seed_link_on_error(*this, ec.message());
    return;
  }

  con->set_open_handler([this](websocketpp::connection_hdl h) {
    this->delegate.seed_link_on_connect(*this);
  });

  con->set_fail_handler([this](websocketpp::connection_hdl h) {
    this->delegate.seed_link_on_error(*this, this->con->get_ec().message());
  });

  con->set_close_handler([this](websocketpp::connection_hdl h) {
    this->delegate.seed_link_on_disconnect(*this);
  });

  con->set_message_handler([this](websocketpp::connection_hdl h, message_ptr msg) {
    std::string msg_str(msg->get_payload());
    this->delegate.seed_link_on_recv(*this, msg_str);
  });

  client.connect(con);
}

void SeedLinkWebsocketNative::disconnect() {
  client.close(con->get_handle(), websocketpp::close::status::normal, "");
}

void SeedLinkWebsocketNative::send(const std::string& data) {
  websocketpp::lib::error_code ec;

  client.send(con->get_handle(), data, websocketpp::frame::opcode::binary, ec);
  if (ec) {
    this->delegate.seed_link_on_error(*this, ec.message());
  }
}
}  // namespace colonio
