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

#include <cstdint>
#include <memory>

namespace colonio {
class ValueImpl;

/**
 * @brief Values in colonio are represented as instance of colino::Value class.
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
    STRING_T   ///< string or byte array
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
   * @brief Construct a new string or byte array Value object.
   *
   * Even if the std::string object contains '\0',
   * it is simply processed as binary data.
   * The data was transferred to other nodes is also processed as is.
   *
   * @param v The source value of the object.
   */
  explicit Value(const std::string& v);
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
   * @tparam T Native type, which corresponds to the value stored by Value object.
   * @return const T& The value stored by Value object.
   */
  template<typename T>
  const T& get() const;

  /**
   * @brief Extract the actual value from the object.
   *
   * @tparam T Native type, which corresponds to the value stored by Value object.
   * @return const T& The value stored by Value object.
   */
  template<typename T>
  T& get();

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
   * @brief Set a new string or byte array for Value object.
   *
   * @param v The source value of the object.
   */
  void set(const std::string& v);

 private:
  friend ValueImpl;
  std::unique_ptr<ValueImpl> impl;
};
}  // namespace colonio
