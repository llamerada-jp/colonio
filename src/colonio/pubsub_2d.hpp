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

#include <colonio/value.hpp>
#include <functional>

namespace colonio {
class Error;

/**
 * @brief Pubsub2D is a module to implement publish–subscribe using 2D coordinate information.
 */
class Pubsub2D {
 public:
  /// let raise an error if no one node that can be received a message within the specified range.
  static const uint32_t RAISE_NO_ONE_RECV = 0x01;

  /**
   * @brief Destroy the Pubsub 2D object
   * You don't need to be aware of the destructor, since module is used as a reference type.
   */
  virtual ~Pubsub2D();

  /**
   * @brief Send a message to the specified coordinates.
   *
   * This method tries to send a message to all nodes within the specified coordinates `x`,`y` and
   * radius `r`. The coordinate of the node on the receiving side is specified by
   * @ref Colonio::set_position. The function set by the ON method with each `name` will be called
   * in the receiver node.
   *
   * - If you send more than one message, the order in which it is received is not guaranteed.
   * - The calculation of coordinates `x`,`y` and radius `r` is based on the coordinate system set by seed.
   * - This method works synchronously, If you want to work asynchronously, use a asynchronous publish method instead.
   *
   * About success/failure, if RAISE_NO_ONE_RECV option is specified,
   * a NO_ONE_RECV error will be returned if there are no nodes in the specified coordinates,
   * or if a message is received by at least one node.
   *
   * @param name Name used to filter messages.
   * @param x The center X coordinate of the message destination.
   * @param y The center Y coordinate of the message destination.
   * @param r The message destination radius.
   * @param value The message to be sent.
   * @param opt Individual options.
   *
   * @sa void publish(
   *     const std::string& name, double x, double y, double r, const Value& value, uint32_t opt,
   *     std::function<void(Pubsub2D&)> on_success, std::function<void(Pubsub2D&, const Error&)> on_failure)
   */
  virtual void publish(
      const std::string& name, double x, double y, double r, const Value& value, uint32_t opt = 0x00) = 0;
  /**
   * @brief Send a message to the specified coordinates asynchronously.
   *
   * The main purpose of the function is the same as @ref publish(const std::string& name, double x, double y, double r,
   * const Value& value, uint32_t opt) This method works asynchronously, and the method returns immediately. Note that
   * the specified callback function is called in a separate thread for processing.
   *
   * @param name Name used to filter messages.
   * @param x The center X coordinate of the message destination.
   * @param y The center Y coordinate of the message destination.
   * @param r The message destination radius.
   * @param value The message to be sent.
   * @param opt Individual options.
   * @param on_success The function will call when success to send the message.
   * @param on_failure The function will call when failure to send the message.
   */
  virtual void publish(
      const std::string& name, double x, double y, double r, const Value& value, uint32_t opt,
      std::function<void(Pubsub2D&)> on_success, std::function<void(Pubsub2D&, const Error&)> on_failure) = 0;

  /**
   * @brief Register a callback function for receiving messages.
   *
   * Register a function to receive messages of the same name sent using the publish method.
   * If this method is called with the same name, only the later registered function is valid.
   * The registered function is called continuously until the @ref off method is used to release it.
   *
   * @param name Name used to filter messages.
   * @param subscriber Subscriber function.
   */
  virtual void on(const std::string& name, const std::function<void(const Value&)>& subscriber) = 0;

  /**
   * @brief Cancel a registered function using the @ref on method.
   *
   * @param name Name used to filter messages.
   */
  virtual void off(const std::string& name) = 0;
};
}  // namespace colonio
