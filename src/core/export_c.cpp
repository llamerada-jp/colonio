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
#include <cstring>
#include <map>
#include <memory>
#include <vector>

#include "colonio/colonio.h"
#include "colonio/colonio.hpp"

using namespace colonio;

static thread_local std::string error_message;
static thread_local std::string error_file;
static thread_local colonio_error_t last_error;

struct kvs_local_data_wrapper {
  std::shared_ptr<std::map<std::string, Value>> cpp_data;
  std::unique_ptr<std::vector<std::string>> keys;
};

struct Wrapper {
  std::unique_ptr<Colonio> colonio;
  std::map<colonio_messaging_writer_t, std::shared_ptr<MessagingResponseWriter>> messaging_writers;
  std::map<colonio_kvs_data_t, kvs_local_data_wrapper> kvs_local_data;
};

colonio_error_t* convert_error(const Error& e);

void colonio_config_set_default(colonio_config_t* config) {
  config->disable_callback_thread = false;
  config->max_user_threads        = 1;
  config->logger_func             = nullptr;
}

colonio_error_t* colonio_init(colonio_t* c, const colonio_config_t* cf) {
  try {
    Wrapper* wrapper = new Wrapper();

    *c = reinterpret_cast<colonio_t>(wrapper);

    ColonioConfig config;
    config.disable_callback_thread = cf->disable_callback_thread;
    config.max_user_threads        = cf->max_user_threads;
    if (cf->logger_func != nullptr) {
      auto f             = cf->logger_func;
      config.logger_func = [c, f](Colonio& _, const std::string& json) {
        f(*c, json.c_str(), json.size() + 1);
      };
    }
    wrapper->colonio = std::unique_ptr<Colonio>(Colonio::new_instance(config));

  } catch (const Error& e) {
    return convert_error(e);
  }

  return nullptr;
}

colonio_error_t* colonio_connect(
    colonio_t c, const char* url, unsigned int url_siz, const char* token, unsigned int token_siz) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  try {
    colonio->connect(std::string(url, url_siz), std::string(token, token_siz));
  } catch (const Error& e) {
    return convert_error(e);
  }

  return nullptr;
}

void colonio_connect_async(
    colonio_t c, const char* url, unsigned int url_siz, const char* token, unsigned int token_siz, void* ptr,
    void (*on_success)(colonio_t, void*), void (*on_failure)(colonio_t, void*, const colonio_error_t*)) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  colonio->connect(
      std::string(url, url_siz), std::string(token, token_siz),
      [c, ptr, on_success](Colonio&) {
        on_success(c, ptr);
      },
      [c, ptr, on_failure](Colonio&, const Error& e) {
        on_failure(c, ptr, convert_error(e));
      });
}

colonio_error_t* colonio_disconnect(colonio_t c) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  try {
    colonio->disconnect();
  } catch (const Error& e) {
    return convert_error(e);
  }

  return nullptr;
}

void colonio_disconnect_async(
    colonio_t c, void* ptr, void (*on_success)(colonio_t, void*),
    void (*on_failure)(colonio_t, void*, const colonio_error_t*)) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  colonio->disconnect(
      [c, ptr, on_success](Colonio&) {
        on_success(c, ptr);
      },
      [c, ptr, on_failure](Colonio&, const Error& e) {
        on_failure(c, ptr, convert_error(e));
      });
}

bool colonio_is_connected(colonio_t c) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  return colonio->is_connected();
}

colonio_error_t* colonio_quit(colonio_t* c) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(*c);

  delete wrapper;
  *c = nullptr;

  return nullptr;
}

void colonio_get_local_nid(colonio_t c, char* dst) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  std::string local_nid = colonio->get_local_nid();
  memcpy(dst, local_nid.c_str(), local_nid.size() + 1);
}

colonio_error_t* colonio_set_position(colonio_t c, double* x, double* y) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  try {
    std::tie(*x, *y) = colonio->set_position(*x, *y);
  } catch (const Error& e) {
    return convert_error(e);
  }

  return nullptr;
}

colonio_error_t* colonio_messaging_post(
    colonio_t c, const char* dst_nid, const char* name, unsigned int name_siz, colonio_const_value_t v, uint32_t opt,
    colonio_value_t* r) {
  Wrapper* wrapper   = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio   = wrapper->colonio.get();
  const Value* value = reinterpret_cast<const Value*>(v);

  try {
    Value result =
        colonio->messaging_post(std::string(dst_nid, COLONIO_NID_LENGTH), std::string(name, name_siz), *value, opt);
    *r = reinterpret_cast<colonio_value_t>(new Value(result));

  } catch (const Error& e) {
    return convert_error(e);
  }

  return nullptr;
}

void colonio_messaging_post_async(
    colonio_t c, const char* dst_nid, const char* name, unsigned int name_siz, colonio_const_value_t v, uint32_t opt,
    void* ptr, void (*on_response)(colonio_t, void*, colonio_const_value_t),
    void (*on_failure)(colonio_t, void*, const colonio_error_t*)) {
  Wrapper* wrapper   = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio   = wrapper->colonio.get();
  const Value* value = reinterpret_cast<const Value*>(v);

  colonio->messaging_post(
      std::string(dst_nid, COLONIO_NID_LENGTH), std::string(name, name_siz), *value, opt,
      [c, ptr, on_response](Colonio&, const Value& value) {
        colonio_const_value_t v = reinterpret_cast<colonio_const_value_t>(&value);
        on_response(c, ptr, v);
      },
      [c, ptr, on_failure](Colonio&, const Error& e) {
        on_failure(c, ptr, convert_error(e));
      });
}

void colonio_messaging_set_handler(
    colonio_t c, const char* name, unsigned int name_siz, void* ptr,
    void (*handler)(colonio_t, void*, const colonio_messaging_request_t*, colonio_messaging_writer_t)) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  colonio->messaging_set_handler(
      std::string(name, name_siz),
      [c, ptr, handler](Colonio&, const MessagingRequest& request, std::shared_ptr<MessagingResponseWriter> writer) {
        Wrapper* wrapper              = reinterpret_cast<Wrapper*>(c);
        std::string src_nid           = request.source_nid.c_str();
        colonio_messaging_request_t r = colonio_messaging_request_t{
            .source_nid = src_nid.c_str(),
            .message    = reinterpret_cast<colonio_const_value_t>(&request.message),
            .options    = request.options,
        };

        colonio_messaging_writer_t w = COLONIO_MESSAGING_WRITER_NONE;
        if (writer) {
          w = reinterpret_cast<colonio_messaging_writer_t>(writer.get());
          wrapper->messaging_writers.insert(std::make_pair(w, writer));
        }

        handler(c, ptr, &r, w);
      });
}

void colonio_messaging_response_writer(colonio_t c, colonio_messaging_writer_t w, colonio_const_value_t message) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);

  if (w == COLONIO_MESSAGING_WRITER_NONE) {
    // TODO warn for unnecessary response
    return;
  }

  auto it = wrapper->messaging_writers.find(w);
  if (it == wrapper->messaging_writers.end()) {
    // TODO warn multiple responses
    return;
  }

  it->second->write(*reinterpret_cast<const Value*>(message));

  wrapper->messaging_writers.erase(it);
}

void colonio_messaging_unset_handler(colonio_t c, const char* name, unsigned int name_siz) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  colonio->messaging_unset_handler(std::string(name, name_siz));
}

void colonio_value_create(colonio_value_t* v) {
  *v = reinterpret_cast<colonio_value_t>(new Value());
}

COLONIO_VALUE_TYPE colonio_value_get_type(colonio_const_value_t v) {
  const Value* value = reinterpret_cast<const Value*>(v);
  return static_cast<COLONIO_VALUE_TYPE>(value->get_type());
}

bool colonio_value_get_bool(colonio_const_value_t v) {
  const Value* value = reinterpret_cast<const Value*>(v);
  return value->get<bool>();
}

int64_t colonio_value_get_int(colonio_const_value_t v) {
  const Value* value = reinterpret_cast<const Value*>(v);
  return value->get<int64_t>();
}

double colonio_value_get_double(colonio_const_value_t v) {
  const Value* value = reinterpret_cast<const Value*>(v);
  return value->get<double>();
}

const char* colonio_value_get_string(colonio_const_value_t v, unsigned int* siz) {
  const Value* value     = reinterpret_cast<const Value*>(v);
  const std::string& str = value->get<std::string>();
  if (siz != nullptr) {
    *siz = str.size();
  }
  return str.c_str();
}

const void* colonio_value_get_binary(colonio_const_value_t v, unsigned int* siz) {
  const Value* value = reinterpret_cast<const Value*>(v);
  *siz               = value->get_binary_size();
  return value->get_binary();
}

void colonio_value_set_bool(colonio_value_t v, bool val) {
  Value* value = reinterpret_cast<Value*>(v);
  value->set(val);
}

void colonio_value_set_int(colonio_value_t v, int64_t val) {
  Value* value = reinterpret_cast<Value*>(v);
  value->set(val);
}

void colonio_value_set_double(colonio_value_t v, double val) {
  Value* value = reinterpret_cast<Value*>(v);
  value->set(val);
}

void colonio_value_set_string(colonio_value_t v, const char* val, unsigned int siz) {
  Value* value = reinterpret_cast<Value*>(v);
  value->set(std::string(val, siz));
}

void colonio_value_set_binary(colonio_value_t v, const void* val, unsigned int siz) {
  // TODO
  assert(false);
}

void colonio_value_free(colonio_value_t* v) {
  Value* value = reinterpret_cast<Value*>(*v);
  delete value;
  *v = nullptr;
}

colonio_kvs_data_t colonio_kvs_get_local_data(colonio_t c) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  std::shared_ptr<std::map<std::string, Value>> cpp_data = colonio->kvs_get_local_data();
  std::unique_ptr<std::vector<std::string>> keys(new std::vector<std::string>());
  for (auto& it : *cpp_data) {
    keys->push_back(it.first);
  }

  colonio_kvs_data_t cur = reinterpret_cast<colonio_kvs_data_t>(cpp_data.get());
  wrapper->kvs_local_data.insert(std::make_pair(
      cur, kvs_local_data_wrapper{
               .cpp_data = cpp_data,
               .keys     = std::move(keys),
           }));
  return cur;
}

void colonio_kvs_get_local_data_async(colonio_t c, void* ptr, void (*handler)(colonio_t, void*, colonio_kvs_data_t)) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  std::unique_ptr<std::vector<std::string>> keys(new std::vector<std::string>());
  Colonio* colonio = wrapper->colonio.get();

  colonio->kvs_get_local_data(
      [c, wrapper, ptr, handler](Colonio&, std::shared_ptr<std::map<std::string, Value>> cpp_data) {
        std::unique_ptr<std::vector<std::string>> keys(new std::vector<std::string>());
        for (auto& it : *cpp_data) {
          keys->push_back(it.first);
        }

        colonio_kvs_data_t cur = reinterpret_cast<colonio_kvs_data_t>(cpp_data.get());
        wrapper->kvs_local_data.insert(std::make_pair(
            cur, kvs_local_data_wrapper{
                     .cpp_data = cpp_data,
                     .keys     = std::move(keys),
                 }));
        handler(c, ptr, cur);
      });
}

unsigned int colonio_kvs_local_data_get_siz(colonio_t c, colonio_kvs_data_t cur) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  auto it          = wrapper->kvs_local_data.find(cur);
  assert(it != wrapper->kvs_local_data.end());
  return it->second.keys->size();
}

const char* colonio_kvs_local_data_get_key(
    colonio_t c, colonio_kvs_data_t data_id, unsigned int idx, unsigned int* siz) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  std::string& key = wrapper->kvs_local_data.at(data_id).keys->at(idx);
  if (siz != nullptr) {
    *siz = key.size();
  }
  return key.c_str();
}

void colonio_kvs_local_data_get_value(colonio_t c, colonio_kvs_data_t data_id, unsigned int idx, colonio_value_t* dst) {
  Wrapper* wrapper                           = reinterpret_cast<Wrapper*>(c);
  kvs_local_data_wrapper& local_data_wrapper = wrapper->kvs_local_data.at(data_id);
  std::string& key                           = local_data_wrapper.keys->at(idx);
  Value& v                                   = local_data_wrapper.cpp_data->at(key);
  *dst                                       = &v;
}

void colonio_kvs_local_data_free(colonio_t c, colonio_kvs_data_t data_id) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  wrapper->kvs_local_data.erase(data_id);
}

colonio_error_t* colonio_kvs_get(colonio_t c, const char* key, unsigned int key_siz, colonio_value_t* dst) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  try {
    Value cpp_value = colonio->kvs_get(std::string(key, key_siz));
    Value* v        = new Value(cpp_value);
    *dst            = reinterpret_cast<colonio_value_t>(v);

  } catch (const Error& e) {
    return convert_error(e);
  }

  return nullptr;
}

void colonio_kvs_get_async(
    colonio_t c, const char* key, unsigned int key_siz, void* ptr,
    void (*on_success)(colonio_t, void*, colonio_const_value_t),
    void (*on_failure)(colonio_t, void*, const colonio_error_t*)) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  colonio->kvs_get(
      std::string(key, key_siz),
      [c, ptr, on_success](Colonio&, const Value& cpp_value) {
        Value* v = new Value(cpp_value);
        on_success(c, ptr, reinterpret_cast<colonio_value_t>(v));
      },
      [c, ptr, on_failure](Colonio&, const Error& e) {
        on_failure(c, ptr, convert_error(e));
      });
}

colonio_error_t* colonio_kvs_set(
    colonio_t c, const char* key, unsigned int key_siz, colonio_const_value_t value, uint32_t opt) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  try {
    const Value* v = reinterpret_cast<const Value*>(value);
    colonio->kvs_set(std::string(key, key_siz), *v, opt);
  } catch (const Error& e) {
    return convert_error(e);
  }

  return nullptr;
}

void colonio_kvs_set_async(
    colonio_t c, const char* key, unsigned int key_siz, colonio_const_value_t value, uint32_t opt, void* ptr,
    void (*on_success)(colonio_t, void*), void (*on_failure)(colonio_t, void*, const colonio_error_t*)) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  const Value* v = reinterpret_cast<const Value*>(value);
  colonio->kvs_set(
      std::string(key, key_siz), *v, opt,
      [c, ptr, on_success](Colonio&) {
        on_success(c, ptr);
      },
      [c, ptr, on_failure](Colonio&, const Error& e) {
        on_failure(c, ptr, convert_error(e));
      });
}

colonio_error_t* colonio_spread_post(
    colonio_t c, double x, double y, double r, const char* name, unsigned int name_siz, colonio_const_value_t message,
    uint32_t opt) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  try {
    colonio->spread_post(x, y, r, std::string(name, name_siz), *reinterpret_cast<const Value*>(message), opt);
  } catch (const Error& e) {
    return convert_error(e);
  }

  return nullptr;
}

void colonio_spread_post_async(
    colonio_t c, double x, double y, double r, const char* name, unsigned int name_siz, colonio_const_value_t message,
    uint32_t opt, void* ptr, void (*on_success)(colonio_t, void*),
    void (*on_failure)(colonio_t, void*, const colonio_error_t*)) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  colonio->spread_post(
      x, y, r, std::string(name, name_siz), *reinterpret_cast<const Value*>(message), opt,
      [c, ptr, on_success](Colonio&) {
        on_success(c, ptr);
      },
      [c, ptr, on_failure](Colonio&, const Error& e) {
        on_failure(c, ptr, convert_error(e));
      });
}

void colonio_spread_set_handler(
    colonio_t c, const char* name, unsigned int name_siz, void* ptr,
    void (*handler)(colonio_t, void*, const colonio_spread_request_t*)) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();

  colonio->spread_set_handler(std::string(name, name_siz), [c, ptr, handler](Colonio&, const SpreadRequest& r) {
    colonio_spread_request_t request = colonio_spread_request_t{
        .source_nid = r.source_nid.c_str(),
        .message    = reinterpret_cast<colonio_const_value_t>(&r.message),
        .options    = r.options,
    };
    handler(c, ptr, &request);
  });
}

void colonio_spread_unset_handler(colonio_t c, const char* name, unsigned int name_siz) {
  Wrapper* wrapper = reinterpret_cast<Wrapper*>(c);
  Colonio* colonio = wrapper->colonio.get();
  colonio->spread_unset_handler(std::string(name, name_siz));
}

colonio_error_t* convert_error(const Error& e) {
  error_message          = e.message;
  error_file             = e.file;
  last_error.fatal       = e.fatal;
  last_error.code        = static_cast<COLONIO_ERROR_CODE>(e.code);
  last_error.message     = error_message.c_str();
  last_error.message_siz = error_message.size();
  last_error.line        = e.line;
  last_error.file        = error_file.c_str();
  last_error.file_siz    = error_file.size();
  return &last_error;
}