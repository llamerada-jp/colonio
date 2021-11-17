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

#include <colonio/definition.hpp>
#include <colonio/error.hpp>
#include <colonio/map.hpp>
#include <colonio/pubsub_2d.hpp>
#include <colonio/value.hpp>
#include <functional>
#include <memory>
#include <string>
#include <tuple>

/**
 * @brief All the functions of colonio are defined in the colonio namespace.
 */
namespace colonio {
/**
 * @brief Main class of colonio. One instance is equivalent to one node.
 */
class Colonio {
 public:
  // options
  static const uint32_t EXPLICIT_EVENT_THREAD      = 0x1;
  static const uint32_t EXPLICIT_CONTROLLER_THREAD = 0x2;
  /**
   * Log messages in JSON format.
   *
   * json["file"]    : Log output file name.
   * json["level"]   : The urgency of the log.
   * json["line"]    : Log output line number.
   * json["message"] : Readable log messages.
   * json["param"]   : The parameters that attached for the log.
   * json["time"]    : Log output time.
   */
  static Colonio* new_instance(
      std::function<void(Colonio&, const std::string&)>&& log_receiver = nullptr, uint32_t opt = 0);

  /**
   * @brief Destroy the Colonio object.
   *
   * Run disconnect if necessary.
   *
   * @sa disconnect()
   */
  virtual ~Colonio();

  /**
   * @brief Get a @ref Map accessor.
   *
   * @param name The assigned name for the accessor.
   * @return Map& Reference for the accessor.
   *
   * **The Map accessor is under development and is likely to change.**
   *
   * Get @ref Map accessor associated with the name.
   * If you get an ACCESSOR with the same name, the reference to the same instance will return.
   * Map's name and other parameters, pre-set to seed, are applied.
   * If accessor is not set, @ref Error will be throw.
   *
   * @sa Map
   */
  virtual Map& access_map(const std::string& name) = 0;

  /**
   * @brief Get a @ref Pubusb2D accessor.
   *
   * @param name The assigned name for the accessor.
   * @return Pubsub2D& Reference for the accessor.
   *
   * Get @ref Pubsub2D accessor associated with the name.
   * If you get an ACCESSOR with the same name, the reference to the same instance will return.
   * Pubsub2D's name and other parameters, pre-set to seed, are applied.
   * If accessor is not set, @ref Error will be throw.
   *
   * @sa Pubsub2D
   */
  virtual Pubsub2D& access_pubsub_2d(const std::string& name) = 0;

  /**
   * @brief Connect to seed and join the cluster.
   *
   * @param url Set the URL of the seed. e.g. "wss://host:1234/path".
   * @param token token does not use. This is an argument for future expansion.
   *
   * Connect to the seed. Also, if there are already other nodes forming a cluster, join them.
   * This method works synchronously and waits for the connection to the cluster to be established.
   * If this method is successful, it will automatically reconnect until disconnect will be called.
   * If you want to work asynchronously, use a asynchronous connect method instead.
   * This method throws @ref Error when an error occurs. At successful completion, nothing is returned.
   *
   * @sa Colonio::connect(const std::string& url, const std::string& token, std::Exce<void(Colonio&)>&& on_success,
   *     std::function<void(Colonio&, const Error&)>&& on_failure)
   */
  virtual void connect(const std::string& url, const std::string& token) = 0;

  /**
   * @brief Connect to seed and join the cluster asynchronously.
   *
   * @param url Set the URL of the seed. e.g. "wss://host:1234/path".
   * @param token token does not use. This is an argument for future expansion.
   * @param on_success The function will call when success to connect.
   * @param on_failure The function will call when failure to connect with error information.
   *
   * The main purpose of the function is the same as @ref connect(const std::string& url, const std::string& token).
   * This method works asynchronously, and the method returns immediately. Note that the specified callback function is
   * called in a separate thread for processing.
   *
   * @sa connect(const std::string& url, const std::string& token)
   */
  virtual void connect(
      const std::string& url, const std::string& token, std::function<void(Colonio&)>&& on_success,
      std::function<void(Colonio&, const Error&)>&& on_failure) = 0;

  /**
   * @brief Disconnect from the cluster and the seed.
   *
   * This method works synchronously, it will wait for disconnecting and finish thread to process.
   * This method must not be used in any other callback in colonio because of stopping the thread and releasing
   * resources.
   * Once a disconnected colonio instance is reused, it is not guaranteed to work.
   * This method throws @ref Error when an error occurs. At successful completion, nothing is returned.
   */
  virtual void disconnect() = 0;
  virtual void disconnect(
      std::function<void(Colonio&)>&& on_success, std::function<void(Colonio&, const Error&)>&& on_failure) = 0;

  virtual bool is_connected() = 0;

  /**
   * @brief Get the node-id of this node.
   *
   * The node-id is unique in the cluster.
   * A new ID will be assigned to the node when connect.
   * Return an empty string if node-id isn't assigned.
   *
   * @return std::string The node-id of this node.
   */
  virtual std::string get_local_nid() = 0;

  /**
   * @brief Sets the current position of the node.
   *
   * This method is only available if Pubsub2d is enabled.
   * The coordinate system depends on the settings in seed.
   * This method works synchronously, If you want to work asynchronously, use a asynchronous set_position method
   * instead.
   *
   * @sa Pubsub2D,
   *     set_position(
   *       double x, double y, std::function<void(Colonio&, double, double)> on_success,
   *       std::function<void(Colonio&, const Error&)> on_failure);
   *
   * @param x Horizontal coordinate.
   * @param y Vertical coordinate.
   * @return std::tuple<double, double> The rounded coordinates will be returned to the input coordinates.
   */
  virtual std::tuple<double, double> set_position(double x, double y) = 0;

  /**
   * @brief Sets the current position of the node asynchronously.
   *
   * The main purpose of the function is the same as @ref set_position(double x, double y).
   * This method works asynchronously, and the method returns immediately. Note that the specified callback function is
   * called in a separate thread for processing.
   *
   * @param x Horizontal coordinate.
   * @param y Vertical coordinate.
   * @param on_success The function will call when success to set the current position.
   * @param on_failure The function will call when failure to set the current position.
   *
   * @sa Pubsub2D,
   *     set_position(double x, double y)
   */
  virtual void set_position(
      double x, double y, std::function<void(Colonio&, double, double)> on_success,
      std::function<void(Colonio&, const Error&)> on_failure) = 0;

  // options
  static const uint32_t SEND_ACCEPT_NEARBY      = 0x01;
  static const uint32_t CONFIRM_RECEIVER_RESULT = 0x02;

  virtual void send(const std::string& dst_nid, const Value& value, uint32_t opt = 0x00) = 0;
  virtual void send(
      const std::string& dst_nid, const Value& value, uint32_t opt, std::function<void(Colonio&)>&& on_success,
      std::function<void(Colonio&, const Error&)>&& on_failure)           = 0;
  virtual void on(std::function<void(Colonio&, const Value&)>&& receiver) = 0;
  virtual void off()                                                      = 0;

  virtual void start_on_event_thread()      = 0;
  virtual void start_on_controller_thread() = 0;

 protected:
  /**
   * @brief Construct a new Colonio object.
   */
  Colonio();

 private:
  Colonio(const Colonio&);
  Colonio& operator=(const Colonio&);
};
}  // namespace colonio
