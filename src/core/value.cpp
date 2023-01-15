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
#include <cassert>

#include "colonio/colonio.hpp"
#include "value_impl.hpp"

namespace colonio {
Value::Value() : impl(std::make_unique<ValueImpl>()) {
}

Value::Value(bool v) : impl(std::make_unique<ValueImpl>(v)) {
}

Value::Value(int64_t v) : impl(std::make_unique<ValueImpl>(v)) {
}

Value::Value(double v) : impl(std::make_unique<ValueImpl>(v)) {
}

Value::Value(const std::string& v) : impl(std::make_unique<ValueImpl>(v)) {
}

Value::Value(const char* v) : impl(std::make_unique<ValueImpl>(std::string(v))) {
}

Value::Value(const Value& src) : impl(std::make_unique<ValueImpl>(*src.impl)) {
}

Value::Value(const void* ptr, std::size_t siz) : impl(std::make_unique<ValueImpl>(ptr, siz)) {
}

Value::~Value() {
}

Value& Value::operator=(const Value& src) {
  impl = std::make_unique<ValueImpl>(*src.impl);

  return *this;
}

bool Value::operator<(const Value& b) const {
  return *impl < *b.impl;
}

// 'get' method.
#define M_GET(CTYPE, VTYPE, STORAGE)       \
  template<>                               \
  const CTYPE& Value::get<CTYPE>() const { \
    assert(get_type() == Value::VTYPE);    \
    return STORAGE;                        \
  }

M_GET(bool, BOOL_T, impl->storage.bool_v)
M_GET(int64_t, INT_T, impl->storage.int64_v)
M_GET(double, DOUBLE_T, impl->storage.double_v)
M_GET(std::string, STRING_T, *impl->storage.string_v)
#undef M_GET

const void* Value::get_binary() const {
  assert(get_type() == Value::BINARY_T);
  return impl->storage.binary_v->data();
}

size_t Value::get_binary_size() const {
  assert(get_type() == Value::BINARY_T);
  return impl->storage.binary_v->size();
}

Value::Type Value::get_type() const {
  return impl->type;
}

void Value::reset() {
  if (get_type() == Value::STRING_T) {
    delete impl->storage.string_v;
  }
  if (get_type() == Value::BINARY_T) {
    delete impl->storage.binary_v;
  }
  impl->type = Value::NULL_T;
  memset(&impl->storage, 0, sizeof(impl->storage));
}

// 'set' method.
#define M_SET(CTYPE, VTYPE, STORAGE)     \
  void Value::set(CTYPE v) {             \
    if (get_type() == Value::STRING_T) { \
      delete impl->storage.string_v;     \
    }                                    \
    if (get_type() == Value::BINARY_T) { \
      delete impl->storage.binary_v;     \
    }                                    \
    impl->type    = Value::VTYPE;        \
    impl->STORAGE = v;                   \
  }

M_SET(bool, BOOL_T, storage.bool_v)
M_SET(int64_t, INT_T, storage.int64_v)
M_SET(double, DOUBLE_T, storage.double_v)
#undef M_SET

void Value::set(const std::string& v) {
  if (get_type() == Value::STRING_T) {
    delete impl->storage.string_v;
  }
  if (get_type() == Value::BINARY_T) {
    delete impl->storage.binary_v;
  }
  impl->type             = Value::STRING_T;
  impl->storage.string_v = new std::string(v);
}

void Value::set(const void* ptr, std::size_t siz) {
  if (get_type() == Value::STRING_T) {
    delete impl->storage.string_v;
  }
  if (get_type() == Value::BINARY_T) {
    delete impl->storage.binary_v;
  }
  impl->type             = Value::BINARY_T;
  impl->storage.binary_v = new std::vector<uint8_t>();
  impl->storage.binary_v->resize(siz);
  std::memcpy(impl->storage.binary_v->data(), ptr, siz);
}
}  // namespace colonio
