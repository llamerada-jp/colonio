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
 * \def COLONIO_HANDLE_FIELDS
 */
#define COLONIO_HANDLE_FIELDS \
  /* public */                \
  void* data;

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
  COLONIO_VALUE_TYPE_STRING
} COLONIO_VALUE_TYPE;

/* ATTENTION: Use same value with another languages. */
#define COLONIO_LOG_LEVEL_INFO "info"
#define COLONIO_LOG_LEVEL_WARN "warn"
#define COLONIO_LOG_LEVEL_ERROR "error"
#define COLONIO_LOG_LEVEL_DEBUG "debug"

/* ATTENTION: Use same value with another languages. */
typedef enum COLONIO_ERROR_CODE {
  COLONIO_ERROR_CODE_UNDEFINED,
  COLONIO_ERROR_CODE_SYSTEM_ERROR,
  COLONIO_ERROR_CODE_CONNECTION_FAILD,
  COLONIO_ERROR_CODE_OFFLINE,
  COLONIO_ERROR_CODE_INCORRECT_DATA_FORMAT,
  COLONIO_ERROR_CODE_CONFLICT_WITH_SETTING,
  COLONIO_ERROR_CODE_NOT_EXIST_KEY,
  COLONIO_ERROR_CODE_EXIST_KEY,
  COLONIO_ERROR_CODE_CHANGED_PROPOSER,
  COLONIO_ERROR_CODE_COLLISION_LATE,
  COLONIO_ERROR_CODE_NO_ONE_RECV,
  COLONIO_ERROR_CODE_CALLBACK_ERROR,
} COLONIO_ERROR_CODE;

#define COLONIO_COLONIO_EXPLICIT_EVENT_THREAD 0x1
#define COLONIO_COLONIO_EXPLICIT_CONTROLLER_THREAD 0x2

#define COLONIO_MAP_ERROR_WITHOUT_EXIST 0x1
#define COLONIO_MAP_ERROR_WITH_EXIST 0x2
/*
#define COLONIO_MAP_TRY_LOCK 0x4
*/

#define COLONIO_PUBSUB_2D_RAISE_NO_ONE_RECV 0x1

typedef struct colonio_s {
  COLONIO_HANDLE_FIELDS
  /* private */
  void* impl;
} colonio_t;

typedef struct colonio_map_s {
  COLONIO_HANDLE_FIELDS
  /* private */
  void* impl;
} colonio_map_t;

typedef struct colonio_pubsub_2d_s {
  COLONIO_HANDLE_FIELDS
  /* private */
  void* impl;
} colonio_pubsub_2d_t;

typedef struct colonio_value_s {
  COLONIO_VALUE_TYPE type;
  union {
    bool bool_v;
    int64_t int_v;
    double double_v;
    struct string_t {
      unsigned int siz;
      char* str;
    } string_v;
  } value;
} colonio_value_t;

typedef struct colonio_error_s {
  COLONIO_ERROR_CODE code;
  const char* message;
  unsigned int message_siz;
} colonio_error_t;

COLONIO_PUBLIC colonio_error_t* colonio_init(
    colonio_t* colonio, void (*logger)(colonio_t*, const char*, unsigned int), uint32_t opt);
COLONIO_PUBLIC colonio_error_t* colonio_connect(
    colonio_t* colonio, const char* url, unsigned int url_siz, const char* token, unsigned int token_siz);
COLONIO_PUBLIC void colonio_connect_async(
    colonio_t* colonio, const char* url, unsigned int url_siz, const char* token, unsigned int token_siz,
    void (*on_success)(colonio_t*), void (*on_failure)(colonio_t*, const colonio_error_t*));
COLONIO_PUBLIC colonio_error_t* colonio_disconnect(colonio_t* colonio);
COLONIO_PUBLIC void colonio_disconnect_async(
    colonio_t* colonio, void (*on_success)(colonio_t*), void (*on_failure)(colonio_t*, const colonio_error_t*));
COLONIO_PUBLIC bool colonio_is_connected(colonio_t* colonio);
COLONIO_PUBLIC colonio_map_t colonio_access_map(colonio_t* colonio, const char* name, unsigned int name_siz);
COLONIO_PUBLIC colonio_pubsub_2d_t
colonio_access_pubsub_2d(colonio_t* colonio, const char* name, unsigned int name_siz);
COLONIO_PUBLIC void colonio_get_local_nid(colonio_t* colonio, char* dst, unsigned int* siz);
COLONIO_PUBLIC colonio_error_t* colonio_set_position(colonio_t* colonio, double* x, double* y);
COLONIO_PUBLIC void colonio_set_position_async(
    colonio_t* colonio, double x, double y, void* ptr, void (*on_success)(colonio_t*, void*, double, double),
    void (*on_failure)(colonio_t*, void*, const colonio_error_t*));
COLONIO_PUBLIC colonio_error_t* colonio_send(
    colonio_t* colonio, const char* dst, unsigned int dst_siz, const colonio_value_t* value, uint32_t opt);
COLONIO_PUBLIC void colonio_send_async(
    colonio_t* colonio, const char* dst, unsigned int dst_siz, const colonio_value_t* value, uint32_t opt, void* ptr,
    void (*on_success)(colonio_t*, void*), void (*on_failure)(colonio_t*, void*, const colonio_error_t*));
COLONIO_PUBLIC void colonio_on(
    colonio_t* colonio, void* ptr, void (*receiver)(colonio_t*, void*, const colonio_value_t*));
COLONIO_PUBLIC void colonio_off(colonio_t* colonio);
COLONIO_PUBLIC void colonio_start_on_event_thread(colonio_t* colonio);
COLONIO_PUBLIC void colonio_start_on_controller_thread(colonio_t* colonio);
COLONIO_PUBLIC colonio_error_t* colonio_quit(colonio_t* colonio);

COLONIO_PUBLIC void colonio_value_init(colonio_value_t* value);
COLONIO_PUBLIC COLONIO_VALUE_TYPE colonio_value_get_type(const colonio_value_t* value);
COLONIO_PUBLIC bool colonio_value_get_bool(colonio_value_t* value);
COLONIO_PUBLIC int64_t colonio_value_get_int(colonio_value_t* value);
COLONIO_PUBLIC double colonio_value_get_double(colonio_value_t* value);
COLONIO_PUBLIC unsigned int colonio_value_get_string_siz(colonio_value_t* value);
COLONIO_PUBLIC void colonio_value_get_string(colonio_value_t* value, char* dst);
COLONIO_PUBLIC void colonio_value_set_bool(colonio_value_t* value, bool v);
COLONIO_PUBLIC void colonio_value_set_int(colonio_value_t* value, int64_t v);
COLONIO_PUBLIC void colonio_value_set_double(colonio_value_t* value, double v);
COLONIO_PUBLIC void colonio_value_set_string(colonio_value_t* value, const char* v, unsigned int siz);
COLONIO_PUBLIC void colonio_value_free(colonio_value_t* value);

COLONIO_PUBLIC colonio_error_t* colonio_map_foreach_local_value(
    colonio_map_t* map, void* ptr,
    void (*func)(colonio_map_t*, void*, const colonio_value_t*, const colonio_value_t*, uint32_t));
COLONIO_PUBLIC colonio_error_t* colonio_map_get(colonio_map_t* map, const colonio_value_t* key, colonio_value_t* dst);
COLONIO_PUBLIC void colonio_map_get_async(
    colonio_map_t* map, const colonio_value_t* key, void* ptr,
    void (*on_success)(colonio_map_t*, void*, const colonio_value_t*),
    void (*on_failure)(colonio_map_t*, void*, const colonio_error_t*));
COLONIO_PUBLIC colonio_error_t* colonio_map_set(
    colonio_map_t* map, const colonio_value_t* key, const colonio_value_t* value, uint32_t opt);
COLONIO_PUBLIC void colonio_map_set_async(
    colonio_map_t* map, const colonio_value_t* key, const colonio_value_t* value, uint32_t opt, void* ptr,
    void (*on_success)(colonio_map_t*, void*), void (*on_failure)(colonio_map_t*, void*, const colonio_error_t*));

COLONIO_PUBLIC colonio_error_t* colonio_pubsub_2d_publish(
    colonio_pubsub_2d_t* pubsub_2d, const char* name, unsigned int name_siz, double x, double y, double r,
    const colonio_value_t* value, uint32_t opt);
COLONIO_PUBLIC void colonio_pubsub_2d_publish_async(
    colonio_pubsub_2d_t* pubsub_2d, const char* name, unsigned int name_siz, double x, double y, double r,
    const colonio_value_t* value, uint32_t opt, void* ptr, void (*on_success)(colonio_pubsub_2d_t*, void*),
    void (*on_failure)(colonio_pubsub_2d_t*, void*, const colonio_error_t*));
COLONIO_PUBLIC void colonio_pubsub_2d_on(
    colonio_pubsub_2d_t* pubsub_2d, const char* name, unsigned int name_siz, void* ptr,
    void (*subscriber)(colonio_pubsub_2d_t*, void*, const colonio_value_t*));
COLONIO_PUBLIC void colonio_pubsub_2d_off(colonio_pubsub_2d_t* pubsub_2d, const char* name, unsigned int name_siz);

/* Undefine macros */
#undef COLONIO_HANDLE_FIELDS
#undef COLONIO_PUBLIC
#undef COLONIO_HIDDEN

/* close extern "C" */
#ifdef __cplusplus
}
#endif

#endif /* #ifndef COLONIO_EXPORT_C_COLONIO_H_ */
