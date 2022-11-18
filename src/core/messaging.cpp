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

#include "messaging.hpp"

#include "colonio/colonio.hpp"
#include "command_manager.hpp"
#include "logger.hpp"
#include "packet.hpp"
#include "value_impl.hpp"

namespace colonio {

class ResponseWriterEmpty : public Colonio::MessagingResponseWriter {
 public:
  ResponseWriterEmpty(Logger& l) : logger(l) {
  }

  void write(const Value&) override {
    log_warn("a response is written but it is not used.");
  }

 private:
  Logger& logger;
};

class ResponseWriterImpl : public Colonio::MessagingResponseWriter {
 public:
  ResponseWriterImpl(Logger& l, CommandManager& c, const Packet& p) :
      logger(l),
      command_manager(c),
      src_nid(p.src_nid),
      id(p.id),
      relay_seed((p.mode & PacketMode::RELAY_SEED) != 0),
      responded(false) {
  }

  virtual ~ResponseWriterImpl() {
    if (responded) {
      return;
    }

    log_warn("no response was written by the handler, and responded null value automatically.");
    write(Value());
  }

  void write(const Value& response) override {
    if (responded) {
      log_warn("multiple responses are written.");
      return;
    }

    std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
    proto::MessagingResponse& param               = *content->mutable_messaging_response();

    ValueImpl::to_pb(param.mutable_response(), response);

    command_manager.send_response(src_nid, id, relay_seed, std::move(content));
    responded = true;
  }

 private:
  Logger& logger;
  CommandManager& command_manager;

  const NodeID src_nid;
  const uint32_t id;
  const bool relay_seed;

  bool responded;
};

Messaging::CommandMessage::CommandMessage(PacketMode::Type mode, Messaging& parent_) :
    Command(mode), logger(parent_.logger), parent(parent_) {
}

void Messaging::CommandMessage::on_response(const Packet& packet) {
  const proto::PacketContent& c = packet.content->as_proto();
  if (!c.has_messaging_response()) {
    log_debug("an unexpected packet received").map("packet", packet);
    Error e = colonio_error(ErrorCode::SYSTEM_UNEXPECTED_PACKET, "an unexpected packet received");
  }

  const proto::MessagingResponse& response = c.messaging_response();
  Value result                             = ValueImpl::from_pb(response.response());
  cb_response(result);
}

void Messaging::CommandMessage::on_error(ErrorCode code, const std::string& message) {
  Error e = colonio_error(code, message);
  cb_failure(e);
}

Messaging::Messaging(Logger& l, CommandManager& c) : logger(l), command_manager(c) {
  command_manager.set_handler(
      proto::PacketContent::kMessaging, std::bind(&Messaging::recv_messaging, this, std::placeholders::_1));
}

Messaging::~Messaging() {
}

void Messaging::post(
    const std::string& dst_nid, const std::string& name, const Value& message, uint32_t opt,
    std::function<void(const Value&)>&& on_response, std::function<void(const Error&)>&& on_failure) {
  std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
  proto::Messaging& param                       = *content->mutable_messaging();
  param.set_opt(opt);
  param.set_name(name);
  ValueImpl::to_pb(param.mutable_message(), message);

  PacketMode::Type mode = 0;
  if ((opt & Colonio::MESSAGING_ACCEPT_NEARBY) == 0) {
    mode |= PacketMode::EXPLICIT;
  }

  if ((opt & Colonio::MESSAGING_IGNORE_RESPONSE) != 0) {
    command_manager.send_packet_one_way(NodeID::from_str(dst_nid), mode, std::move(content));
    on_response(Value());

  } else {
    std::shared_ptr<CommandMessage> command = std::make_shared<CommandMessage>(mode, *this);
    command->name                           = name;
    command->cb_response                    = on_response;
    command->cb_failure                     = on_failure;

    command_manager.send_packet(NodeID::from_str(dst_nid), std::move(content), command);
  }
}

void Messaging::set_handler(
    const std::string& name,
    std::function<
        void(std::shared_ptr<const Colonio::MessagingRequest>, std::shared_ptr<Colonio::MessagingResponseWriter>)>&&
        handler) {
  handlers[name] = handler;
}

void Messaging::unset_handler(const std::string& name) {
  handlers.erase(name);
}

void Messaging::recv_messaging(const Packet& packet) {
  proto::Messaging content                           = packet.content->as_proto().messaging();
  const std::string& name                            = content.name();
  std::shared_ptr<Colonio::MessagingRequest> request = std::make_shared<Colonio::MessagingRequest>();
  request->source                                    = packet.src_nid;
  request->message                                   = ValueImpl::from_pb(content.message());
  request->options                                   = content.opt();

  auto it_handler = handlers.find(name);
  if (it_handler == handlers.end()) {
    if ((request->options & Colonio::MESSAGING_IGNORE_RESPONSE) == 0) {
      log_debug("handler not found").map("name", name);
      command_manager.send_error(packet, ErrorCode::MESSAGING_HANDLER_NOT_FOUND, "handler `" + name + "` not found.");
    }
    return;
  }

  auto& handler = it_handler->second;
  if ((request->options & Colonio::MESSAGING_IGNORE_RESPONSE) != 0) {
    handler(request, std::shared_ptr<Colonio::MessagingResponseWriter>(new ResponseWriterEmpty(logger)));

  } else {
    handler(
        request,
        std::shared_ptr<Colonio::MessagingResponseWriter>(new ResponseWriterImpl(logger, command_manager, packet)));
  }
}
}  //  namespace colonio
