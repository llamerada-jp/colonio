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
  void (*cb_on_output_log)(colonio_t*, COLONIO_LOG_LEVEL, const char*, unsigned int);

  ColonioC() : colonio(nullptr), cb_on_output_log(nullptr) {
  }

 protected:
  void on_output_log(colonio::LogLevel level, const std::string& message) override {
    if (cb_on_output_log != nullptr) {
      cb_on_output_log(colonio, static_cast<COLONIO_LOG_LEVEL>(level), message.c_str(), message.size());
    }
  }
};
}  // namespace colonio_export_c

static thread_local std::string error_message;
static thread_local colonio_error_t last_error;

colonio_error_t* convert_error(const colonio::Error& e);
colonio_error_t* convert_exception(const colonio::Exception& e);
void convert_value_c_to_cpp(colonio::Value* dst, const colonio_value_t* src);
void convert_value_cpp_to_c(colonio_value_t* dst, const colonio::Value* src);

colonio_error_t* colonio_init(colonio_t* colonio) {
  memset(colonio, 0, sizeof(colonio_t));
  colonio->impl                    = new colonio_export_c::ColonioC();
  colonio_export_c::ColonioC* impl = nullptr;

  try {
    impl          = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);
    impl->colonio = colonio;

  } catch (const colonio::Exception& e) {
    return convert_exception(e);
  }

  return nullptr;
}

colonio_error_t* colonio_connect(
    colonio_t* colonio, const char* url, unsigned int url_siz, const char* token, unsigned int token_siz) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);

  try {
    impl->connect(std::string(url, url_siz), std::string(token, token_siz));
  } catch (const colonio::Exception& e) {
    return convert_exception(e);
  }

  return nullptr;
}

void colonio_connect_async(
    colonio_t* colonio, const char* url, unsigned int url_siz, const char* token, unsigned int token_siz,
    void (*on_success)(colonio_t*), void (*on_failure)(colonio_t*, const colonio_error_t*)) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);
  impl->connect(
      std::string(url, url_siz), std::string(token, token_siz),
      [colonio, on_success](colonio::Colonio&) { on_success(colonio); },
      [colonio, on_failure](colonio::Colonio&, const colonio::Error& e) { on_failure(colonio, convert_error(e)); });
}

colonio_error_t* colonio_disconnect(colonio_t* colonio) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);

  try {
    impl->disconnect();
  } catch (const colonio::Exception& e) {
    return convert_exception(e);
  }

  delete impl;
  colonio->impl = nullptr;

  return nullptr;
}

colonio_map_t colonio_access_map(colonio_t* colonio, const char* name, unsigned int name_siz) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);

  colonio_map_t map;
  map.data = colonio->data;
  map.impl = &impl->access_map(std::string(name, name_siz));

  return map;
}

colonio_pubsub_2d_t colonio_access_pubsub_2d(colonio_t* colonio, const char* name, unsigned int name_siz) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);

  colonio_pubsub_2d_t pubsub_2d;
  pubsub_2d.data = colonio->data;
  pubsub_2d.impl = &impl->access_pubsub_2d(std::string(name, name_siz));

  return pubsub_2d;
}

void colonio_get_local_nid(colonio_t* colonio, char* dst, unsigned int* siz) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);
  std::string local_nid            = impl->get_local_nid();
  memcpy(dst, local_nid.c_str(), local_nid.size() + 1);
  if (siz != nullptr) {
    *siz = local_nid.size();
  }
}

colonio_error_t* colonio_set_position(colonio_t* colonio, double* x, double* y) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);

  try {
    std::tie(*x, *y) = impl->set_position(*x, *y);
  } catch (const colonio::Exception& e) {
    return convert_exception(e);
  }

  return nullptr;
}

void colonio_set_position_async(
    colonio_t* colonio, double x, double y, void* ptr, void (*on_success)(colonio_t*, void*, double, double),
    void (*on_failure)(colonio_t*, void*, const colonio_error_t*)) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);
  impl->set_position(
      x, y,
      [colonio, ptr, on_success](colonio::Colonio&, double app_x, double app_y) {
        on_success(colonio, ptr, app_x, app_y);
      },
      [colonio, ptr, on_failure](colonio::Colonio&, const colonio::Error& e) {
        on_failure(colonio, ptr, convert_error(e));
      });
}

void colonio_set_on_output_log(
    colonio_t* colonio, void (*func)(colonio_t*, COLONIO_LOG_LEVEL, const char*, unsigned int)) {
  colonio_export_c::ColonioC* impl = reinterpret_cast<colonio_export_c::ColonioC*>(colonio->impl);
  impl->cb_on_output_log           = func;
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

unsigned int colonio_value_get_string_siz(colonio_value_t* value) {
  assert(value->type == COLONIO_VALUE_TYPE_STRING);
  return value->value.string_v.siz;
}

void colonio_value_get_string(colonio_value_t* value, char* dst) {
  assert(value->type == COLONIO_VALUE_TYPE_STRING);
  memcpy(dst, value->value.string_v.str, value->value.string_v.siz);
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

void colonio_value_set_string(colonio_value_t* value, const char* v, unsigned int siz) {
  colonio_value_free(value);
  value->type               = COLONIO_VALUE_TYPE_STRING;
  value->value.string_v.str = new char[siz];
  memcpy(value->value.string_v.str, v, siz);
  value->value.string_v.siz = siz;
}

void colonio_value_free(colonio_value_t* value) {
  if (value->type == COLONIO_VALUE_TYPE_STRING) {
    delete[] value->value.string_v.str;
  }
  colonio_value_init(value);
}

colonio_error_t* colonio_map_get(colonio_map_t* map, const colonio_value_t* key, colonio_value_t* dst) {
  colonio::Map* impl = reinterpret_cast<colonio::Map*>(map->impl);
  colonio::Value cpp_key;
  convert_value_c_to_cpp(&cpp_key, key);

  try {
    colonio::Value cpp_value = impl->get(cpp_key);
    convert_value_cpp_to_c(dst, &cpp_value);
  } catch (const colonio::Exception& e) {
    return convert_exception(e);
  }

  return nullptr;
}

void colonio_map_get_async(
    colonio_map_t* map, const colonio_value_t* key, void* ptr,
    void (*on_success)(colonio_map_t*, void*, const colonio_value_t*),
    void (*on_failure)(colonio_map_t*, void*, const colonio_error_t*)) {
  colonio::Map* impl = reinterpret_cast<colonio::Map*>(map->impl);
  colonio::Value cpp_key;
  convert_value_c_to_cpp(&cpp_key, key);

  impl->get(
      cpp_key,
      [map, ptr, on_success](colonio::Map&, const colonio::Value& value) {
        colonio_value_t c_value;
        colonio_value_init(&c_value);
        convert_value_cpp_to_c(&c_value, &value);
        on_success(map, ptr, &c_value);
        colonio_value_free(&c_value);
      },
      [map, ptr, on_failure](colonio::Map&, const colonio::Error& e) { on_failure(map, ptr, convert_error(e)); });
}

colonio_error_t* colonio_map_set(
    colonio_map_t* map, const colonio_value_t* key, const colonio_value_t* value, uint32_t opt) {
  colonio::Map* impl = reinterpret_cast<colonio::Map*>(map->impl);
  colonio::Value cpp_key;
  colonio::Value cpp_value;
  convert_value_c_to_cpp(&cpp_key, key);
  convert_value_c_to_cpp(&cpp_value, value);

  try {
    impl->set(cpp_key, cpp_value, opt);
  } catch (const colonio::Exception& e) {
    return convert_exception(e);
  }

  return nullptr;
}

void colonio_map_set_async(
    colonio_map_t* map, const colonio_value_t* key, const colonio_value_t* value, uint32_t opt, void* ptr,
    void (*on_success)(colonio_map_t*, void*), void (*on_failure)(colonio_map_t*, void*, const colonio_error_t*)) {
  colonio::Map* impl = reinterpret_cast<colonio::Map*>(map->impl);
  colonio::Value cpp_key;
  colonio::Value cpp_value;
  convert_value_c_to_cpp(&cpp_key, key);
  convert_value_c_to_cpp(&cpp_value, value);

  impl->set(
      cpp_key, cpp_value, opt, [map, ptr, on_success](colonio::Map&) { on_success(map, ptr); },
      [map, ptr, on_failure](colonio::Map&, const colonio::Error& e) { on_failure(map, ptr, convert_error(e)); });
}

colonio_error_t* colonio_pubsub_2d_publish(
    colonio_pubsub_2d_t* pubsub_2d, const char* name, unsigned int name_siz, double x, double y, double r,
    const colonio_value_t* value, uint32_t opt) {
  colonio::Pubsub2D* impl = reinterpret_cast<colonio::Pubsub2D*>(pubsub_2d->impl);
  colonio::Value cpp_value;
  convert_value_c_to_cpp(&cpp_value, value);

  try {
    impl->publish(std::string(name, name_siz), x, y, r, cpp_value, opt);
  } catch (const colonio::Exception& e) {
    return convert_exception(e);
  }

  return nullptr;
}

void colonio_pubsub_2d_publish_async(
    colonio_pubsub_2d_t* pubsub_2d, const char* name, unsigned int name_siz, double x, double y, double r,
    const colonio_value_t* value, uint32_t opt, void* ptr, void (*on_success)(colonio_pubsub_2d_t*, void*),
    void (*on_failure)(colonio_pubsub_2d_t*, void*, const colonio_error_t*)) {
  colonio::Pubsub2D* impl = reinterpret_cast<colonio::Pubsub2D*>(pubsub_2d->impl);
  colonio::Value cpp_value;
  convert_value_c_to_cpp(&cpp_value, value);

  impl->publish(
      std::string(name, name_siz), x, y, r, cpp_value, opt,
      [pubsub_2d, ptr, on_success](colonio::Pubsub2D&) { on_success(pubsub_2d, ptr); },
      [pubsub_2d, ptr, on_failure](colonio::Pubsub2D&, const colonio::Error& e) {
        on_failure(pubsub_2d, ptr, convert_error(e));
      });
}

void colonio_pubsub_2d_on(
    colonio_pubsub_2d_t* pubsub_2d, const char* name, unsigned int name_siz, void* ptr,
    void (*subscriber)(colonio_pubsub_2d_t* pubsub_2d, void* ptr, const colonio_value_t* value)) {
  colonio::Pubsub2D* impl = reinterpret_cast<colonio::Pubsub2D*>(pubsub_2d->impl);
  impl->on(std::string(name, name_siz), [pubsub_2d, ptr, subscriber](const colonio::Value& value) {
    colonio_value_t c_value;
    colonio_value_init(&c_value);
    convert_value_cpp_to_c(&c_value, &value);
    subscriber(pubsub_2d, ptr, &c_value);
    colonio_value_free(&c_value);
  });
}

void colonio_pubsub_2d_off(colonio_pubsub_2d_t* pubsub_2d, const char* name, unsigned int name_siz) {
  colonio::Pubsub2D* impl = reinterpret_cast<colonio::Pubsub2D*>(pubsub_2d->impl);
  impl->off(std::string(name, name_siz));
}

colonio_error_t* convert_error(const colonio::Error& e) {
  error_message      = e.message;
  last_error.code    = static_cast<COLONIO_ERROR_CODE>(e.code);
  last_error.message = error_message.c_str();
  return &last_error;
}

colonio_error_t* convert_exception(const colonio::Exception& e) {
  error_message      = e.message;
  last_error.code    = static_cast<COLONIO_ERROR_CODE>(e.code);
  last_error.message = error_message.c_str();
  return &last_error;
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
      dst->set(std::string(src->value.string_v.str, src->value.string_v.siz));
    } break;
  }
}

void convert_value_cpp_to_c(colonio_value_t* dst, const colonio::Value* src) {
  colonio_value_free(dst);
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
