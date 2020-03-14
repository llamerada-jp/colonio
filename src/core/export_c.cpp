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

#include <cassert>
#include <cstring>

#include "colonio/colonio.h"
#include "colonio/colonio.hpp"

namespace colonio_export_c {
class ColonioC : public colonio::Colonio {
 public:
  colonio_t* colonio;
  void (*cb_on_require_invoke)(colonio_t*, unsigned int);
  void (*cb_on_output_log)(colonio_t*, COLONIO_LOG_LEVEL, const char*, unsigned int);
  void (*cb_on_debug_event)(colonio_t*, COLONIO_DEBUG_EVENT, const char*, unsigned int);

  ColonioC() : colonio(nullptr), cb_on_require_invoke(nullptr), cb_on_output_log(nullptr), cb_on_debug_event(nullptr) {
  }

  unsigned int _invoke() {
    // return invoke();
  }

 protected:
  /*
   void on_require_invoke(unsigned int msec) override {
     assert(cb_on_require_invoke != nullptr);
     cb_on_require_invoke(colonio, msec);
   }
   */

  void on_output_log(colonio::LogLevel::Type level, const std::string& message) override {
    if (cb_on_output_log != nullptr) {
      cb_on_output_log(colonio, static_cast<COLONIO_LOG_LEVEL>(level), message.c_str(), message.size() + 1);
    }
  }

  void on_debug_event(colonio::DebugEvent::Type event, const std::string& json) override {
    if (cb_on_debug_event != nullptr) {
      cb_on_debug_event(colonio, static_cast<COLONIO_DEBUG_EVENT>(event), json.c_str(), json.size() + 1);
    }
  }
};
}  // namespace colonio_export_c

void convert_value_c_to_cpp(colonio::Value* dst, const colonio_value_t* src);
void convert_value_cpp_to_c(colonio_value_t* dst, const colonio::Value* src);

void colonio_init(colonio_t* colonio, void (*on_require_invoke)(colonio_t*, unsigned int)) {
  memset(colonio, 0, sizeof(colonio_t));
  colonio->impl = new colonio_export_c::ColonioC();

  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);
  impl->colonio                    = colonio;
  impl->cb_on_require_invoke       = on_require_invoke;
}

void colonio_connect(
    colonio_t* colonio, const char* url, const char* token, void (*on_success)(colonio_t*),
    void (*on_failure)(colonio_t*)) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);
  assert(false);
  /* TODO
  impl->connect(
      url, token, [on_success, colonio](colonio::Colonio&) { on_success(colonio); },
      [on_failure, colonio](colonio::Colonio&) { on_failure(colonio); });
      */
}

void colonio_disconnect(colonio_t* colonio) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);
  impl->disconnect();
  delete impl;
  colonio->impl = nullptr;
}

colonio_map_t colonio_access_map(colonio_t* colonio, const char* name) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);

  colonio_map_t map;
  map.data = colonio->data;
  map.impl = &impl->access_map(name);

  return map;
}

colonio_pubsub_2d_t colonio_access_pubsub_2d(colonio_t* colonio, const char* name) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);

  colonio_pubsub_2d_t pubsub_2d;
  pubsub_2d.data = colonio->data;
  pubsub_2d.impl = &impl->access_pubsub_2d(name);

  return pubsub_2d;
}

void colonio_get_local_nid(colonio_t* colonio, char* dest) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);
  std::string local_nid            = impl->get_local_nid();
  memcpy(dest, local_nid.c_str(), local_nid.size() + 1);
}

void colonio_set_position(colonio_t* colonio, double x, double y) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);
  impl->set_position(x, y);
}

void colonio_set_on_output_log(
    colonio_t* colonio, void (*func)(colonio_t*, COLONIO_LOG_LEVEL, const char*, unsigned int)) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);
  impl->cb_on_output_log           = func;
}

void colonio_set_on_debug_event(
    colonio_t* colonio, void (*func)(colonio_t*, COLONIO_DEBUG_EVENT, const char*, unsigned int)) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);
  impl->cb_on_debug_event          = func;
}

unsigned int colonio_invoke(colonio_t* colonio) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);
  return impl->_invoke();
}

void colonio_value_init(colonio_value_t* value) {
  memset(value, 0, sizeof(colonio_value_t));
  value->type = COLONIO_VALUE_TYPE_NULL;
}

COLONIO_VALUE_TYPE colonio_value_get_type(const colonio_value_t* value) {
  return value->type;
}

bool colonio_value_get_bool(colonio_value_t* value) {
  assert(value->type == COLONIO_VALUE_TYPE_BOOL);
  return value->value.bool_v;
}

int64_t colonio_value_get_int(colonio_value_t* value) {
  assert(value->type == COLONIO_VALUE_TYPE_INT);
  return value->value.int_v;
}

double colonio_value_get_double(colonio_value_t* value) {
  assert(value->type == COLONIO_VALUE_TYPE_DOUBLE);
  return value->value.double_v;
}

unsigned int colonio_value_get_string_len(colonio_value_t* value) {
  assert(value->type == COLONIO_VALUE_TYPE_STRING);
  return value->value.string_v.len;
}

void colonio_value_get_string(colonio_value_t* value, char* dest) {
  assert(value->type == COLONIO_VALUE_TYPE_STRING);
  memcpy(dest, value->value.string_v.str, value->value.string_v.len);
}

void colonio_value_set_bool(colonio_value_t* value, bool v) {
  colonio_value_free(value);
  value->type         = COLONIO_VALUE_TYPE_BOOL;
  value->value.bool_v = v;
}

void colonio_value_set_int(colonio_value_t* value, int64_t v) {
  colonio_value_free(value);
  value->type        = COLONIO_VALUE_TYPE_INT;
  value->value.int_v = v;
}

void colonio_value_set_double(colonio_value_t* value, double v) {
  colonio_value_free(value);
  value->type           = COLONIO_VALUE_TYPE_DOUBLE;
  value->value.double_v = v;
}

void colonio_value_set_string(colonio_value_t* value, const char* v, unsigned int len) {
  colonio_value_free(value);
  value->type               = COLONIO_VALUE_TYPE_STRING;
  value->value.string_v.str = new char[len];
  memcpy(value->value.string_v.str, v, len);
  value->value.string_v.len = len;
}

void colonio_value_free(colonio_value_t* value) {
  if (value->type == COLONIO_VALUE_TYPE_STRING) {
    delete[] value->value.string_v.str;
  }
  colonio_value_init(value);
}

void colonio_map_get(
    colonio_map_t* map, const colonio_value_t* key, void* ptr,
    void (*on_success)(colonio_map_t* map, void* ptr, const colonio_value_t* v),
    void (*on_failure)(colonio_map_t* map, void* ptr, COLONIO_MAP_FAILURE_REASON reason)) {
  colonio::Map* impl = reinterpret_cast<colonio::Map*>(map->impl);
  colonio::Value cpp_key;
  convert_value_c_to_cpp(&cpp_key, key);
  assert(false);
  /* TODO
  impl->get(
      cpp_key,
      [map, ptr, on_success](const colonio::Value& value) {
        colonio_value_t c_value;
        convert_value_cpp_to_c(&c_value, &value);
        on_success(map, ptr, &c_value);
        colonio_value_free(&c_value);
      },
      [map, ptr, on_failure](colonio::Exception::Code reason) {
        on_failure(map, ptr, static_cast<COLONIO_MAP_FAILURE_REASON>(reason));
      });
      */
}

void colonio_map_set(
    colonio_map_t* map, const colonio_value_t* key, const colonio_value_t* value, void* ptr,
    void (*on_success)(colonio_map_t* map, void* ptr),
    void (*on_failure)(colonio_map_t* map, void* ptr, COLONIO_MAP_FAILURE_REASON reason), int opt) {
  colonio::Map* impl = reinterpret_cast<colonio::Map*>(map->impl);
  colonio::Value cpp_key;
  colonio::Value cpp_value;
  convert_value_c_to_cpp(&cpp_key, key);
  convert_value_c_to_cpp(&cpp_value, value);

  assert(false);
  /* TODO
  impl->set(
      cpp_key, cpp_value, [map, ptr, on_success]() { on_success(map, ptr); },
      [map, ptr, on_failure](colonio::Exception::Code reason) {
        on_failure(map, ptr, static_cast<COLONIO_MAP_FAILURE_REASON>(reason));
      },
      opt);
      */
}

void colonio_pubsub_2d_publish(
    colonio_pubsub_2d_t* pubsub_2d, const char* name, unsigned int name_siz, double x, double y, double r,
    const colonio_value_t* value, void* ptr, void (*on_success)(colonio_pubsub_2d_t* pubsub_2d, void* ptr),
    void (*on_failure)(colonio_pubsub_2d_t* pubsub_2d, void* ptr, COLONIO_PUBSUB_2D_FAILURE_REASON reason)) {
  colonio::Pubsub2D* impl = reinterpret_cast<colonio::Pubsub2D*>(pubsub_2d->impl);
  colonio::Value cpp_value;
  convert_value_c_to_cpp(&cpp_value, value);
  /* TODO
  impl->publish(
      std::string(name, name_siz), x, y, r, cpp_value, [pubsub_2d, ptr, on_success]() { on_success(pubsub_2d, ptr); },
      [pubsub_2d, ptr, on_failure](colonio::Exception::Code reason) {
        on_failure(pubsub_2d, ptr, static_cast<COLONIO_PUBSUB_2D_FAILURE_REASON>(reason));
      });
      */
}

void colonio_pubsub_2d_on(
    colonio_pubsub_2d_t* pubsub_2d, const char* name, unsigned int name_siz, void* ptr,
    void (*subscriber)(colonio_pubsub_2d_t* pubsub_2d, void* ptr, const colonio_value_t* value)) {
  colonio::Pubsub2D* impl = reinterpret_cast<colonio::Pubsub2D*>(pubsub_2d->impl);
  impl->on(std::string(name, name_siz), [pubsub_2d, ptr, subscriber](const colonio::Value& value) {
    colonio_value_t c_value;
    convert_value_cpp_to_c(&c_value, &value);
    subscriber(pubsub_2d, ptr, &c_value);
    colonio_value_free(&c_value);
  });
}

void colonio_pubsub_2d_off(colonio_pubsub_2d_t* pubsub_2d, const char* name, unsigned int name_siz) {
  colonio::Pubsub2D* impl = reinterpret_cast<colonio::Pubsub2D*>(pubsub_2d->impl);
  impl->off(std::string(name, name_siz));
}

void convert_value_c_to_cpp(colonio::Value* dst, const colonio_value_t* src) {
  switch (colonio_value_get_type(src)) {
    case COLONIO_VALUE_TYPE_NULL: {
      dst->reset();
    } break;

    case COLONIO_VALUE_TYPE_BOOL: {
      dst->set(src->value.bool_v);
    } break;

    case COLONIO_VALUE_TYPE_INT: {
      dst->set(src->value.int_v);
    } break;

    case COLONIO_VALUE_TYPE_DOUBLE: {
      dst->set(src->value.double_v);
    } break;

    case COLONIO_VALUE_TYPE_STRING: {
      dst->set(std::string(src->value.string_v.str, src->value.string_v.len));
    } break;
  }
}

void convert_value_cpp_to_c(colonio_value_t* dst, const colonio::Value* src) {
  switch (src->get_type()) {
    case colonio::Value::NULL_T: {
      colonio_value_free(dst);
    } break;

    case colonio::Value::BOOL_T: {
      colonio_value_set_bool(dst, src->get<bool>());
    } break;

    case colonio::Value::INT_T: {
      colonio_value_set_int(dst, src->get<int64_t>());
    } break;

    case colonio::Value::DOUBLE_T: {
      colonio_value_set_double(dst, src->get<double>());
    } break;

    case colonio::Value::STRING_T: {
      const std::string& s = src->get<std::string>();
      colonio_value_set_string(dst, s.c_str(), s.size());
    } break;
  }
}
