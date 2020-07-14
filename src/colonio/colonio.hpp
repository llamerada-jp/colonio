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

#include <colonio/definition.hpp>
#include <colonio/error.hpp>
#include <colonio/exception.hpp>
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
  /**
   * @brief Construct a new Colonio object.
   */
  Colonio();

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
   * If accessor is not set, @ref Exception will be throw.
   *
   * @sa Map
   */
  Map& access_map(const std::string& name);

  /**
   * @brief Get a @ref Pubusb2D accessor.
   *
   * @param name The assigned name for the accessor.
   * @return Pubsub2D& Reference for the accessor.
   *
   * Get @ref Pubsub2D accessor associated with the name.
   * If you get an ACCESSOR with the same name, the reference to the same instance will return.
   * Pubsub2D's name and other parameters, pre-set to seed, are applied.
   * If accessor is not set, @ref Exception will be throw.
   *
   * @sa Pubsub2D
   */
  Pubsub2D& access_pubsub_2d(const std::string& name);

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
   * This method throws @ref Exception when an error occurs. At successful completion, nothing is returned.
   *
   * @sa Colonio::connect(const std::string& url, const std::string& token, std::function<void(Colonio&)> on_success,
   *     std::function<void(Colonio&, const Error&)> on_failure)
   */
  void connect(const std::string& url, const std::string& token);

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
  void connect(
      const std::string& url, const std::string& token, std::function<void(Colonio&)> on_success,
      std::function<void(Colonio&, const Error&)> on_failure);

#ifndef EMSCRIPTEN
  /**
   * @brief Disconnect from the cluster and the seed.
   *
   * This method works synchronously, it will wait for disconnecting and finish thread to process.
   * This method must not be used in any other callback in colonio because of stopping the thread and releasing
   * resources.
   * Once a disconnected colonio instance is reused, it is not guaranteed to work.
   * This method throws @ref Exception when an error occurs. At successful completion, nothing is returned.
   */
  void disconnect();
#else
  void disconnect(std::function<void(Colonio&)> on_success, std::function<void(Colonio&, const Error&)> on_failure);
#endif

  /**
   * @brief Get the node-id of this node.
   *
   * The node-id is unique in the cluster.
   * A new ID will be assigned to the node when connect.
   * Return an empty string if node-id isn't assigned.
   *
   * @return std::string The node-id of this node.
   */
  std::string get_local_nid();

  /**
   * @brief Sets the current position of the node.
   *
   * This method is only available if Pubsub2d is enabled.
   * The coordinate system depends on the settings in seed.
   * This method works synchronously, If you want to work asynchronously, use a asynchronous set_position method
   * instead.
   *
   * @param x Horizontal coordinate.
   * @param y Vertical coordinate.
   * @return std::tuple<double, double> The rounded coordinates will be returned to the input coordinates.
   *
   * @sa Pubsub2D,
   *     set_position(
   *       double x, double y, std::function<void(Colonio&, double, double)> on_success,
   *       std::function<void(Colonio&, const Error&)> on_failure);
   */
  std::tuple<double, double> set_position(double x, double y);

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
  void set_position(
      double x, double y, std::function<void(Colonio&, double, double)> on_success,
      std::function<void(Colonio&, const Error&)> on_failure);

 protected:
  /**
   * @brief You can set the log output destination by overriding this method.
   *
   * json["file"]    : Log output file name.
   * json["level"]   : The urgency of the log.
   * json["line"]    : Log output line number.
   * json["message"] : Readable log messages.
   * json["param"]   : The parameters that attached for the log.
   * json["time"]    : Log output time.
   *
   * This method is executed in a specific thread and must be exclusive if required.
   *
   * @param json Log messages in JSON format.
   */
  virtual void on_output_log(const std::string& json);

 private:
  class Impl;
  std::unique_ptr<Impl> impl;

  Colonio(const Colonio&);
  Colonio& operator=(const Colonio&);
};
}  // namespace colonio
