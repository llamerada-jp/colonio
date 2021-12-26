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
  // These options are provided for implementations of other languages or libraries. And are not normally used.
  /// This declares that the thread for the event loop will be specified explicitly instead of created internally.
  static const uint32_t EXPLICIT_EVENT_THREAD = 0x1;
  /// This declares that the thread for the colonio main loop will be specified explicitly instead of created
  /// internally.
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
  Colonio(const Colonio&) = delete;
  Colonio& operator=(const Colonio&) = delete;

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

  // Options for `call_by_nid` method.
  /// If there is no node with a matching node-id, the node with the closest node-id will receive the call.
  static const uint32_t CALL_ACCEPT_NEARBY = 0x01;
  /// If this option is specified, call will not wait for a response. Also, no error will occur if no node receives the
  /// call. You should return null value instead of this option if you just don't need return value.
  static const uint32_t CALL_IGNORE_REPLY = 0x02;

  /**
   * @brief Call remote procedure on or neer the destination node.
   *
   * This method provides the simple feature of RPC with a value.
   *
   * @param dst_nid Target node's ID.
   * @param name A name to identify the procedure.
   * @param value A value to be sent with the call.
   * @param opt Options.
   *
   * @sa call_by_nid(
   *       const std::string& dst_nid, const std::string& name, const Value& value, uint32_t opt,
   *       std::function<void(Colonio&, const Value&)>&& on_success,
   *       std::function<void(Colonio&, const Error&)>&& on_failure)
   * @sa on_call(const std::string& name, std::function<Value(Colonio&, const CallParameter&)>&& func)
   * @sa off_call(const std::string& name)
   */
  virtual Value call_by_nid(
      const std::string& dst_nid, const std::string& name, const Value& value, uint32_t opt = 0x00) = 0;

  /**
   * @brief Call remote procedure on or neer the destination node asynchronously.
   *
   * This method provides the simple feature of RPC with a value.
   *
   * @param dst_nid Target node's ID.
   * @param name A name to identify the procedure.
   * @param value A value to be sent.
   * @param opt Options.
   * @param on_success The function will call when success to send the message.
   * @param on_failure The function will call when failure to send the message.
   *
   * @sa call_by_nid(const std::string& dst_nid, const std::string& name, const Value& value, uint32_t opt)
   * @sa on_call(const std::string& name, std::function<Value(Colonio&, const CallParameter&)>&& func)
   * @sa off_call(const std::string& name)
   */
  virtual void call_by_nid(
      const std::string& dst_nid, const std::string& name, const Value& value, uint32_t opt,
      std::function<void(Colonio&, const Value&)>&& on_success,
      std::function<void(Colonio&, const Error&)>&& on_failure) = 0;

  struct CallParameter {
    const std::string name;
    const Value value;
    const uint32_t opt;
  };

  /**
   * @brief Register a callback function to receive messages from the call_by_nid method.
   *
   * If another function has already been registered, that registration will be overwritten.
   *
   * @param name A name to identify the procedure.
   * @param func Receiver function.
   */
  virtual void on_call(const std::string& name, std::function<Value(Colonio&, const CallParameter&)>&& func) = 0;

  /**
   * @brief Release the function registered in the on_call method.
   *
   * @param name A name to identify the procedure.
   */
  virtual void off_call(const std::string& name) = 0;

  /**
   * @brief This is the method to call inside the thread for events.
   *
   * This method is provided for implementations of other languages or libraries. And are not normally used.
   * Used with the EXPLICIT_EVENT_THREAD option.
   */
  virtual void start_on_event_thread() = 0;

  /**
   * @brief This is the method to call inside the thread for colonio main loop.
   *
   * This method is provided for implementations of other languages or libraries. And are not normally used.
   * Used with the EXPLICIT_CONTROLLER_THREAD option.
   */
  virtual void start_on_controller_thread() = 0;

 protected:
  /**
   * @brief Construct a new Colonio object.
   */
  Colonio();
};
}  // namespace colonio
