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
#include "core/value_impl.hpp"

#include <cassert>

#include "colonio.pb.h"
#include "convert.hpp"

namespace colonio {
ValueImpl::ValueImpl() : type(Value::NULL_T) {
  memset(&storage, 0, sizeof(Storage));
}

ValueImpl::ValueImpl(bool v) : type(Value::BOOL_T) {
  memset(&storage, 0, sizeof(Storage));
  storage.bool_v = v;
}

ValueImpl::ValueImpl(int64_t v) : type(Value::INT_T) {
  memset(&storage, 0, sizeof(Storage));
  storage.int64_v = v;
}

ValueImpl::ValueImpl(double v) : type(Value::DOUBLE_T) {
  memset(&storage, 0, sizeof(Storage));
  storage.double_v = v;
}

ValueImpl::ValueImpl(const std::string& v) : type(Value::STRING_T) {
  memset(&storage, 0, sizeof(Storage));
  storage.string_v = new std::string(v);
}

ValueImpl::ValueImpl(const void* v, unsigned int siz) : type(Value::BINARY_T) {
  memset(&storage, 0, sizeof(Storage));
  storage.binary_v = new std::vector<uint8_t>();
}

ValueImpl::ValueImpl(const ValueImpl& src) : type(src.type) {
  memset(&storage, 0, sizeof(Storage));

  if (src.type == Value::STRING_T) {
    storage.string_v = new std::string(*src.storage.string_v);

  } else if (src.type == Value::BINARY_T) {
    storage.binary_v = new std::vector<uint8_t>(*src.storage.binary_v);

  } else {
    storage = src.storage;
  }
}

ValueImpl::~ValueImpl() {
  if (type == Value::STRING_T) {
    delete storage.string_v;
    type             = Value::NULL_T;
    storage.string_v = nullptr;
  }
  if (type == Value::BINARY_T) {
    delete storage.binary_v;
    type             = Value::NULL_T;
    storage.binary_v = nullptr;
  }
}

void ValueImpl::to_pb(proto::Value* pb, const Value& value) {
  switch (value.impl->type) {
    case Value::NULL_T:
      // pb->clear_value();
      pb->Clear();
      break;

    case Value::BOOL_T:
      pb->set_bool_v(value.impl->storage.bool_v);
      break;

    case Value::INT_T:
      pb->set_int_v(value.impl->storage.int64_v);
      break;

    case Value::DOUBLE_T:
      pb->set_double_v(value.impl->storage.double_v);
      break;

    case Value::STRING_T:
      pb->set_string_v(*(value.impl->storage.string_v));
      break;

    case Value::BINARY_T:
      pb->set_binary_v(
          std::string(reinterpret_cast<char*>(&value.impl->storage.binary_v[0]), value.impl->storage.binary_v->size()));
      break;

    default:
      assert(false);
      // pb->clear_value();
      pb->Clear();
  }
}

Value ValueImpl::from_pb(const proto::Value& pb) {
  switch (pb.value_case()) {
    case proto::Value::VALUE_NOT_SET:
      return Value();

    case proto::Value::kBoolV:
      return Value(pb.bool_v());

    case proto::Value::kIntV:
      return Value(pb.int_v());

    case proto::Value::kDoubleV:
      return Value(pb.double_v());

    case proto::Value::kStringV:
      return Value(pb.string_v());

    case proto::Value::kBinaryV:
      return Value(reinterpret_cast<const void*>(pb.binary_v().c_str()), pb.binary_v().size());

    default:
      assert(false);
      return Value();
  }
}

std::string ValueImpl::to_str(const Value& value) {
  switch (value.impl->type) {
    case Value::BOOL_T:
      return value.impl->storage.bool_v ? "true" : "false";

    case Value::INT_T:
      return Convert::int2str(value.impl->storage.int64_v);

    case Value::DOUBLE_T:
      return std::to_string(value.impl->storage.double_v);

    case Value::STRING_T:
      return std::string("\"") + *(value.impl->storage.string_v) + std::string("\"");

    case Value::BINARY_T:
      assert(false);
      return std::string("binary");

    default:
      assert(value.impl->type == Value::NULL_T);
      return std::string("null");
  }
}

bool ValueImpl::operator<(const ValueImpl& b) const {
  if (type != b.type) {
    return type < b.type;

  } else {
    switch (type) {
      case Value::BOOL_T:
        return storage.bool_v < b.storage.bool_v;

      case Value::INT_T:
        return storage.int64_v < b.storage.int64_v;

      case Value::DOUBLE_T:
        return storage.double_v < b.storage.double_v;

      case Value::STRING_T:
        return *(storage.string_v) < *(b.storage.string_v);

      default:
        assert(type == Value::NULL_T);
        return false;
    }
  }
}
}  // namespace colonio
