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
#include <memory>

namespace colonio {
class ValueImpl;
class Value {
 public:
  enum Type {
    NULL_T,
    BOOL_T,
    INT_T,
    DOUBLE_T,
    STRING_T
  };

  Value();
  Value(const Value& src);
  explicit Value(bool v);
  explicit Value(int64_t v);
  explicit Value(double v);
  explicit Value(const std::string& v);
  virtual ~Value();

  Value& operator=(const Value& src);
  bool operator<(const Value& b) const;

  template <typename T> const T& get() const;
  template <typename T> T& get();
  Type get_type() const;
  void reset();
  void set(bool v);
  void set(int64_t v);
  void set(double v);
  void set(const std::string& v);

 private:
  friend ValueImpl;
  std::unique_ptr<ValueImpl> impl;
};
}  // namespace colonio
