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

#include <cstddef>
#include <functional>
#include <map>
#include <memory>
#include <string>
#include <tuple>

/**
 * @brief All the functions of colonio are defined in the colonio namespace.
 */
namespace colonio {
class Colonio;
class ValueImpl;

/**
 * @brief The levels determine the importance of the message.
 */
namespace LogLevel {
const std::string ERROR("error");
const std::string WARN("warn");
const std::string INFO("info");
const std::string DEBUG("debug");
}  // namespace LogLevel

/**
 * @brief ErrorCode is assigned each error reason and is used with Error and Exception.
 *
 * @sa Error, Exception
 */
enum class ErrorCode : unsigned int {
  UNDEFINED,                     ///< Undefined error is occurred.
  SYSTEM_INCORRECT_DATA_FORMAT,  ///< Incorrect data format detected.
  SYSTEM_CONFLICT_WITH_SETTING,  ///< The calling method or setting parameter was inconsistent with the configuration in
                                 ///< the seed.
  CONNECTION_FAILED,             ///< An error on connection start failure.
  CONNECTION_OFFLINE,            ///< The node cannot perform processing because of offline.

  PACKET_NO_ONE_RECV,  ///< There was no node receiving the message.
  PACKET_TIMEOUT,      ///< An error occurs when timeout.

  MESSAGING_HANDLER_NOT_FOUND,  /// An error that occur when message sent to a non-existent handler.

  KVS_NOT_FOUND,  ///< Tried to get a value for a key that doesn't exist.
  KVS_PROHIBIT_OVERWRITE,
  KVS_COLLISION,

  SPREAD_NO_ONE_RECEIVE,
};

/**
 * @brief Error information. This is used when the asynchronous method calls a failed callback and is thrown when an
 * error occurs in a synchronous method.
 *
 *
 * @sa ErrorCode
 */
class Error : public std::exception {
 public:
  /// True if the error is fatal and the process of colonio can not continue.
  const bool fatal;
  /// Code to indicate the cause of the error.
  const ErrorCode code;
  /// A detailed message string for display or bug report.
  const std::string message;
  /// The line number where the exception was thrown (for debug).
  const unsigned long line;
  /// The file name where the exception was thrown (for debug).
  const std::string file;

  /**
   * @brief Construct a new Error object.
   *
   * @param fatal True if the error is fatal and the process of colonio can not continue.
   * @param code Code to indicate the cause of the error.
   * @param message A detailed message string for display or bug report.
   * @param line The line number where the exception was thrown.
   * @param file The file name where the exception was thrown.
   */
  Error(bool fatal, ErrorCode code, const std::string& message, int line, const std::string& file);

  /**
   * @brief Override the standard method to output message.
   *
   * @return const char* The message is returned as it is.
   */
  const char* what() const noexcept override;
};

/**
 * @brief Values in colonio are represented as instance of colonio::Value class.
 */
class Value {
 public:
  /**
   * @brief It represents the type that the Value has.
   */
  enum Type {
    NULL_T,    ///< null
    BOOL_T,    ///< boolean
    INT_T,     ///< integer
    DOUBLE_T,  ///< float number
    STRING_T,  ///< string(UTF8 is expected in C/C++)
    BINARY_T   ///< binary
  };

  Value();

  /**
   * @brief Copy construct a new Value object.
   *
   * @param src The copy source object.
   */
  Value(const Value& src);

  /**
   * @brief Construct a new boolean Value object.
   *
   * @param v The source value of the object.
   */
  explicit Value(bool v);

  /**
   * @brief Construct a new integer Value object.
   *
   * @param v The source value of the object.
   */
  explicit Value(int64_t v);

  /**
   * @brief Construct a new float number Value object.
   *
   * @param v The source value of the object.
   */
  explicit Value(double v);

  /**
   * @brief Construct a new string Value object.
   *
   * The char array is converted using the std::string constructor,
   * so up to the first `\0` is used to create a Value object.
   *
   * @param v The source value of the object.
   */
  explicit Value(const char* v);

  /**
   * @brief Construct a new string Value object.
   *
   * The data was transferred to other nodes is also processed as is.
   *
   * @param v The source value of the object.
   */
  explicit Value(const std::string& v);

  /**
   * @brief Construct a new Value object as a binary type.
   *
   * @param ptr
   * @param len Size of binary data.
   */
  Value(const void* ptr, std::size_t siz);

  /**
   * @brief Destroy the Value object
   *
   */
  virtual ~Value();

  /**
   * @brief Copy operation.
   *
   * @param src The copy source object.
   * @return Value&
   */
  Value& operator=(const Value& src);

  /**
   * @brief Implementation of comparison operations for std::map and other algorithms.
   *
   * This comparison operation should not be used as a semantic order in a user program.
   *
   * @param b The object to be compared.
   * @return true
   * @return false
   */
  bool operator<(const Value& b) const;

  /**
   * @brief Extract the actual value from the object.
   *
   * The value is passed as a reference type, which will be changed by
   * the call to \ref reset and \ref set method.
   * The value may be changed by the implementation of the module, such as a setter of another node.
   * Also, depending on the implementation of the module,
   * the value may be changed by another node's setter, etc.
   * Do not hold the returned values as reference types or pointers.
   *
   * @param T Native type, which corresponds to the value stored by Value object.
   * @return const T& The value stored by Value object.
   */
  template<typename T>
  const T& get() const;

  const void* get_binary() const;

  std::size_t get_binary_size() const;

  /**
   * @brief Get the type stored by Value object.
   *
   * @return Type The type stored by Value object.
   */
  Type get_type() const;

  /**
   * @brief Reset the value stored by Value to null.
   */
  void reset();

  /**
   * @brief Set a new boolean value for Value object.
   *
   * @param v The source value of the object.
   */
  void set(bool v);

  /**
   * @brief Set a new integer value for Value object.
   *
   * @param v The source value of the object.
   */
  void set(int64_t v);

  /**
   * @brief Set a new float number value for Value object.
   *
   * @param v The source value of the object.
   */
  void set(double v);

  /**
   * @brief Set a new string for Value object.
   *
   * @param v The source value of the object.
   */
  void set(const std::string& v);

  /**
   * @brief Set a new binary data for Value object.
   *
   * @param ptr
   * @param len
   */
  void set(const void* ptr, std::size_t siz);

 private:
  friend ValueImpl;
  std::unique_ptr<ValueImpl> impl;
};

class ColonioConfig {
 public:
  ColonioConfig();

  /**
   * @brief `disable_callback_thread` is a switch to disable creating new threads for exec callback.
   *
   * The callback functions are exec on Colonio's main thread.
   * The callback function must not block the thread. And should be lightweight. Otherwise, the main thread will be
   * blocked, and the whole Colonio process will be slow or freeze. This option is for developing wrappers for JS,
   * Golang, and others. You should not use this option if you don't know the internal structure of Colonio.
   *
   * default: false
   */
  bool disable_callback_thread;

  /**
   * @brief `disable_seed_verification` is a switch to disable SSL certificate verification for the seed.
   *
   * This switch is only for testing or developing. It is strongly recommended that this switch be set to off when
   * publishing services.
   *
   * default: false
   */
  bool disable_seed_verification;

  /**
   * @brief `max_user_threads` describes the maximum number of user threads.
   *
   * Callback functions of Colonio will be run on user threads when `disable_callback_thread` is false. User threads are
   * created as needed up to this variable. The developer should set this value to create enough threads when callback
   * functions depend on the other callback functions' return.
   *
   * default: 1
   */
  unsigned int max_user_threads;

  /**
   * @brief `session_timeout_ms` describes the timeout of session between node with the seed.
   *
   * After timeout from last translate between the seed, the session infomation will be cleared and will be send
   * authenticate packet before other packet to the seed.
   *
   * default: 30000
   */
  unsigned int seed_session_timeout_ms;

  /**
   * @brief `logger` is for customizing the the log receiver.
   *
   * `logger` method call when output log. You should implement this method to customize log. Colonio outputs 4 types
   * log, `info`, `warn`, `error`, and `debug`. Default logger output info level log to stdout, others are output to
   * stderr. You can customize it by using this interface. Note that the method could be call by multi threads. You
   * have to implement thread-safe if necessary. Log messages past from colonio are JSON format having keys below.
   *
   * file    : Log output file name.
   * level   : The urgency of the log.
   * line    : Log output line number.
   * message : Readable log messages.
   * param   : The parameters that attached for the log.
   * time    : Log output time.
   *
   * @param json JSON formatted log message.
   */
  std::function<void(Colonio& colonio, const std::string& json)> logger_func;
};

// Options for `messaging_post` method.
/// If there is no node with a matching node-id, the node with the closest node-id will receive the message.
static const uint32_t MESSAGING_ACCEPT_NEARBY = 0x01;
/// If this option is specified, the post method does not wait a response or error message. You get null value as
/// response. No error returns even if the destination node does not exist.
static const uint32_t MESSAGING_IGNORE_RESPONSE = 0x02;

struct MessagingRequest {
  std::string source_nid;
  Value message;
  uint32_t options;
};

class MessagingResponseWriter {
 public:
  virtual ~MessagingResponseWriter();
  virtual void write(const Value&) = 0;
};

static const uint32_t KVS_MUST_EXIST_KEY     = 0x01;  // del, unlock
static const uint32_t KVS_PROHIBIT_OVERWRITE = 0x02;  // set

/// let raise an error if no one node that can be received a message within the specified range.
static const uint32_t SPREAD_SOMEONE_MUST_RECEIVE = 0x01;

struct SpreadRequest {
  std::string source_nid;
  Value message;
  uint32_t options;
};

/**
 * @brief Main class of colonio. One instance is equivalent to one node.
 */
class Colonio {
 public:
  static Colonio* new_instance(ColonioConfig& config);

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
   * @brief Connect to seed and join the cluster.
   *
   * @param url Set the URL of the seed. e.g. "https://host:1234/path".
   * @param token token does not use. This is an argument for future expansion.
   *
   * Connect to the seed. Also, if there are already other nodes forming a cluster, join them.
   * This method works synchronously and waits for the connection to the cluster to be established.
   * If this method is successful, it will automatically reconnect until disconnect will be called.
   * If you want to work asynchronously, use a asynchronous connect method instead.
   * This method throws @ref Error when an error occurs. At successful completion, nothing is returned.
   *
   * @sa Colonio::connect(const std::string& url, const std::string& token, std::Exec<void(Colonio&)>&& on_success,
   *     std::function<void(Colonio&, const Error&)>&& on_failure)
   */
  virtual void connect(const std::string& url, const std::string& token) = 0;

  /**
   * @brief Connect to seed and join the cluster asynchronously.
   *
   * @param url Set the URL of the seed. e.g. "https://host:1234/path".
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
   * This method is only available if the coordinate system is enabled.
   * The coordinate system depends on the settings in seed.
   * This method works synchronously, but other nodes know the position of this node asynchronously.
   *
   * @param x Horizontal coordinate.
   * @param y Vertical coordinate.
   * @return std::tuple<double, double> The rounded coordinates will be returned to the input coordinates.
   */
  virtual std::tuple<double, double> set_position(double x, double y) = 0;

  /**
   * @brief Post a message for the destination node and wait response message.
   *
   * This method provides the simple feature like RPC.
   *
   * @param dst_nid Target node's ID.
   * @param name
   * @param message
   * @param opt Options.
   */
  virtual Value messaging_post(
      const std::string& dst_nid, const std::string& name, const Value& message, uint32_t opt = 0x0) = 0;

  /**
   * @brief Post a message for the destination node and get response message asynchronously.
   *
   * @param dst_nid Target node's ID.
   * @param name
   * @param message
   * @param opt Options.
   * @param on_response A callback will be called when get response message.
   * @param on_failure A callback will be called when an error occurred.
   */
  virtual void messaging_post(
      const std::string& dst_nid, const std::string& name, const Value& message, uint32_t opt,
      std::function<void(Colonio&, const Value&)>&& on_response,
      std::function<void(Colonio&, const Error&)>&& on_failure) = 0;

  /**
   * @brief register the handler for
   *
   * If another handler has already been registered, the old one will be overwritten.
   *
   * @param name A name to identify the handler.
   * @param handler The handler function. null if the received message does not require response.
   */
  virtual void messaging_set_handler(
      const std::string& name,
      std::function<void(Colonio&, const MessagingRequest&, std::shared_ptr<MessagingResponseWriter>)>&& handler) = 0;

  /**
   * @brief Release the function registered in the on_call method.
   *
   * @param name A name to identify the handler.
   */
  virtual void messaging_unset_handler(const std::string& name) = 0;

  virtual std::shared_ptr<std::map<std::string, Value>> kvs_get_local_data() = 0;
  virtual void kvs_get_local_data(
      std::function<void(Colonio&, std::shared_ptr<std::map<std::string, Value>>)> handler) = 0;
  virtual Value kvs_get(const std::string& key)                                             = 0;
  virtual void kvs_get(
      const std::string& key, std::function<void(Colonio&, const Value&)>&& on_success,
      std::function<void(Colonio&, const Error&)>&& on_failure)                        = 0;
  virtual void kvs_set(const std::string& key, const Value& value, uint32_t opt = 0x0) = 0;
  virtual void kvs_set(
      const std::string& key, const Value& value, uint32_t opt, std::function<void(Colonio&)>&& on_success,
      std::function<void(Colonio&, const Error&)>&& on_failure) = 0;

  virtual void spread_post(
      double x, double y, double r, const std::string& name, const Value& message, uint32_t opt = 0x0) = 0;
  virtual void spread_post(
      double x, double y, double r, const std::string& name, const Value& message, uint32_t opt,
      std::function<void(Colonio&)>&& on_success, std::function<void(Colonio&, const Error&)>&& on_failure) = 0;
  virtual void spread_set_handler(
      const std::string& name, std::function<void(Colonio&, const SpreadRequest&)>&& handler) = 0;
  virtual void spread_unset_handler(const std::string& name)                                  = 0;

 protected:
  /**
   * @brief Construct a new Colonio object.
   */
  Colonio();
};
}  // namespace colonio
