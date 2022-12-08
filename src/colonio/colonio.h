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
#ifndef COLONIO_EXPORT_C_COLONIO_H_
#define COLONIO_EXPORT_C_COLONIO_H_

#include <stdbool.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/**
 * \def COLONIO_PUBLIC
 * API design for C++(日本語訳:C++のためのAPIデザイン)
 */
#if defined _WIN32 || defined __CYGWIN__
#  ifdef COLONIO_WITH_EXPORTING /* define COLONIO_WITH_EXPORTING for generate dll */
#    ifdef __GNUC__
#      define COLONIO_PUBLIC __attribute__((dllexport))
#    else
#      define COLONIO_PUBLIC __declspec(dllexport)
#    endif
#  else
#    ifdef __GNUC__
#      define COLONIO_PUBLIC __attribute__(dllimport))
#    else
#      define COLONIO_PUBLIC __declspec(dllimport)
#    endif
#  endif
#else
#  if __GNUC__ >= 4
#    define COLONIO_PUBLIC __attribute__((visibility("default")))
#    define COLONIO_HIDDEN __attribute__((visibility("hidden")))
#  else
#    define COLONIO_PUBLIC
#    define COLONIO_HIDDEN
#  endif
#endif

/**
 * \define COLONIO_NID_LENGTH
 * String length of node-id without terminator '\0'.
 */
#define COLONIO_NID_LENGTH 32
/* ATTENTION: Use same value with another languages. */
typedef enum COLONIO_VALUE_TYPE {
  COLONIO_VALUE_TYPE_NULL,
  COLONIO_VALUE_TYPE_BOOL,
  COLONIO_VALUE_TYPE_INT,
  COLONIO_VALUE_TYPE_DOUBLE,
  COLONIO_VALUE_TYPE_STRING,
  COLONIO_VALUE_TYPE_BINARY
} COLONIO_VALUE_TYPE;

/* ATTENTION: Use same value with another languages. */
#define COLONIO_LOG_LEVEL_INFO "info"
#define COLONIO_LOG_LEVEL_WARN "warn"
#define COLONIO_LOG_LEVEL_ERROR "error"
#define COLONIO_LOG_LEVEL_DEBUG "debug"

/* ATTENTION: Use same value with another languages. */
typedef enum COLONIO_ERROR_CODE {
  COLONIO_ERROR_CODE_UNDEFINED,
  COLONIO_ERROR_CODE_SYSTEM_INCORRECT_DATA_FORMAT,
  COLONIO_ERROR_CODE_SYSTEM_CONFLICT_WITH_SETTING,
  COLONIO_ERROR_CODE_CONNECTION_FAILED,
  COLONIO_ERROR_CODE_CONNECTION_OFFLINE,
  COLONIO_ERROR_CODE_PACKET_NO_ONE_RECV,
  COLONIO_ERROR_CODE_PACKET_TIMEOUT,
  COLONIO_ERROR_CODE_MESSAGING_HANDLER_NOT_FOUND,
  COLONIO_ERROR_CODE_KVS_NOT_FOUND,
  COLONIO_ERROR_CODE_KVS_PROHIBIT_OVERWRITE,
  COLONIO_ERROR_CODE_KVS_COLLISION,
  COLONIO_ERROR_CODE_SPREAD_NO_ONE_RECEIVE,
} COLONIO_ERROR_CODE;

#define COLONIO_MESSAGING_ACCEPT_NEARBY 0x01
#define COLONIO_MESSAGING_IGNORE_RESPONSE 0x02

#define COLONIO_KVS_MUST_EXIST_KEY 0x01
#define COLONIO_KVS_PROHIBIT_OVERWRITE 0x02

#define COLONIO_SPREAD_SOMEONE_MUST_RECEIVE 0x01

typedef void* colonio_t;
typedef void* colonio_value_t;
typedef const void* colonio_const_value_t;

typedef struct colonio_error_s {
  bool fatal;
  COLONIO_ERROR_CODE code;
  const char* message;
  unsigned int message_siz;
  unsigned int line;
  const char* file;
  unsigned int file_siz;
} colonio_error_t;

typedef struct colonio_config_s {
  bool disable_callback_thread;
  unsigned int max_user_threads;
  void (*logger_func)(colonio_t, const char*, unsigned int);
} colonio_config_t;

COLONIO_PUBLIC void colonio_config_set_default(colonio_config_t* config);
COLONIO_PUBLIC colonio_error_t* colonio_init(colonio_t* c, const colonio_config_t* config);
COLONIO_PUBLIC colonio_error_t* colonio_connect(
    colonio_t c, const char* url, unsigned int url_siz, const char* token, unsigned int token_siz);
COLONIO_PUBLIC void colonio_connect_async(
    colonio_t c, const char* url, unsigned int url_siz, const char* token, unsigned int token_siz, void* ptr,
    void (*on_success)(colonio_t, void*), void (*on_failure)(colonio_t, void*, const colonio_error_t*));
COLONIO_PUBLIC colonio_error_t* colonio_disconnect(colonio_t c);
COLONIO_PUBLIC void colonio_disconnect_async(
    colonio_t c, void* ptr, void (*on_success)(colonio_t, void*),
    void (*on_failure)(colonio_t, void*, const colonio_error_t*));
COLONIO_PUBLIC bool colonio_is_connected(colonio_t c);
COLONIO_PUBLIC colonio_error_t* colonio_quit(colonio_t* c);
COLONIO_PUBLIC void colonio_get_local_nid(colonio_t c, char* dst);
COLONIO_PUBLIC colonio_error_t* colonio_set_position(colonio_t c, double* x, double* y);

typedef struct colonio_messaging_request_s {
  const char* source_nid;
  colonio_const_value_t message;
  uint32_t options;
} colonio_messaging_request_t;

typedef void* colonio_messaging_writer_t;
#define COLONIO_MESSAGING_WRITER_NONE nullptr

COLONIO_PUBLIC colonio_error_t* colonio_messaging_post(
    colonio_t c, const char* dst_nid, const char* name, unsigned int name_siz, colonio_const_value_t v, uint32_t opt,
    colonio_value_t* result);
COLONIO_PUBLIC void colonio_messaging_post_async(
    colonio_t c, const char* dst_nid, const char* name, unsigned int name_siz, colonio_const_value_t v, uint32_t opt,
    void* ptr, void (*on_response)(colonio_t, void*, colonio_const_value_t),
    void (*on_failure)(colonio_t, void*, const colonio_error_t*));
COLONIO_PUBLIC void colonio_messaging_set_handler(
    colonio_t c, const char* name, unsigned int name_siz, void* ptr,
    void (*handler)(colonio_t, void*, const colonio_messaging_request_t*, colonio_messaging_writer_t));
COLONIO_PUBLIC void colonio_messaging_response_writer(
    colonio_t c, colonio_messaging_writer_t w, colonio_const_value_t message);
COLONIO_PUBLIC void colonio_messaging_unset_handler(colonio_t c, const char* name, unsigned int name_siz);

typedef void* colonio_kvs_data_t;

COLONIO_PUBLIC colonio_kvs_data_t colonio_kvs_get_local_data(colonio_t c);
COLONIO_PUBLIC void colonio_kvs_get_local_data_async(
    colonio_t c, void* ptr, void (*handler)(colonio_t, void*, colonio_kvs_data_t));
COLONIO_PUBLIC unsigned int colonio_kvs_local_data_get_siz(colonio_t c, colonio_kvs_data_t cur);
COLONIO_PUBLIC const char* colonio_kvs_local_data_get_key(
    colonio_t c, colonio_kvs_data_t cur, unsigned int idx, unsigned int* siz);
COLONIO_PUBLIC void colonio_kvs_local_data_get_value(
    colonio_t c, colonio_kvs_data_t cur, unsigned int idx, colonio_value_t* dst);
COLONIO_PUBLIC void colonio_kvs_local_data_free(colonio_t c, colonio_kvs_data_t cur);
COLONIO_PUBLIC colonio_error_t* colonio_kvs_get(
    colonio_t c, const char* key, unsigned int key_siz, colonio_value_t* dst);
COLONIO_PUBLIC void colonio_kvs_get_async(
    colonio_t c, const char* key, unsigned int key_siz, void* ptr,
    void (*on_success)(colonio_t, void*, colonio_const_value_t),
    void (*on_failure)(colonio_t, void*, const colonio_error_t*));
COLONIO_PUBLIC colonio_error_t* colonio_kvs_set(
    colonio_t c, const char* key, unsigned int key_siz, colonio_const_value_t value, uint32_t opt);
COLONIO_PUBLIC void colonio_kvs_set_async(
    colonio_t c, const char* key, unsigned int key_siz, colonio_const_value_t value, uint32_t opt, void* ptr,
    void (*on_success)(colonio_t, void*), void (*on_failure)(colonio_t, void*, const colonio_error_t*));

typedef struct colonio_spread_request_s {
  const char* source_nid;
  colonio_const_value_t message;
  uint32_t options;
} colonio_spread_request_t;

COLONIO_PUBLIC colonio_error_t* colonio_spread_post(
    colonio_t c, double x, double y, double r, const char* name, unsigned int name_siz, colonio_const_value_t message,
    uint32_t opt);
COLONIO_PUBLIC void colonio_spread_post_async(
    colonio_t c, double x, double y, double r, const char* name, unsigned int name_siz, colonio_const_value_t message,
    uint32_t opt, void* ptr, void (*on_success)(colonio_t, void*),
    void (*on_failure)(colonio_t, void*, const colonio_error_t*));
COLONIO_PUBLIC void colonio_spread_set_handler(
    colonio_t c, const char* name, unsigned int name_siz, void* ptr,
    void (*handler)(colonio_t, void*, const colonio_spread_request_t*));
COLONIO_PUBLIC void colonio_spread_unset_handler(colonio_t c, const char* name, unsigned int name_siz);

COLONIO_PUBLIC void colonio_value_create(colonio_value_t* v);
COLONIO_PUBLIC COLONIO_VALUE_TYPE colonio_value_get_type(colonio_const_value_t v);
COLONIO_PUBLIC bool colonio_value_get_bool(colonio_const_value_t v);
COLONIO_PUBLIC int64_t colonio_value_get_int(colonio_const_value_t v);
COLONIO_PUBLIC double colonio_value_get_double(colonio_const_value_t v);
COLONIO_PUBLIC const char* colonio_value_get_string(colonio_const_value_t v, unsigned int* siz);
COLONIO_PUBLIC const void* colonio_value_get_binary(colonio_const_value_t v, unsigned int* siz);
COLONIO_PUBLIC void colonio_value_set_bool(colonio_value_t v, bool val);
COLONIO_PUBLIC void colonio_value_set_int(colonio_value_t v, int64_t val);
COLONIO_PUBLIC void colonio_value_set_double(colonio_value_t v, double val);
COLONIO_PUBLIC void colonio_value_set_string(colonio_value_t v, const char* val, unsigned int siz);
COLONIO_PUBLIC void colonio_value_set_binary(colonio_value_t v, const void* val, unsigned int siz);
COLONIO_PUBLIC void colonio_value_free(colonio_value_t* v);

/* Undefine macros */
#undef COLONIO_PUBLIC
#undef COLONIO_HIDDEN

/* close extern "C" */
#ifdef __cplusplus
}
#endif

#endif /* #ifndef COLONIO_EXPORT_C_COLONIO_H_ */
