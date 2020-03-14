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
#  ifdef COLONIO_WITH_EXPORTING /* -DCOLONIO_WITH_EXPORTING for generate dll */
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
typedef enum COLONIO_LOG_LEVEL {
  COLONIO_LOG_LEVEL_INFO,
  COLONIO_LOG_LEVEL_ERROR,
  COLONIO_LOG_LEVEL_DEBUG
} COLONIO_LOG_LEVEL;

/* ATTENTION: Use same value with another languages. */
typedef enum COLONIO_DEBUG_EVENT {
  COLONIO_DEBUG_EVENT_MAP_SET,
  COLONIO_DEBUG_EVENT_LINKS,
  COLONIO_DEBUG_EVENT_NEXTS,
  COLONIO_DEBUG_EVENT_POSITION,
  COLONIO_DEBUG_EVENT_REQUIRED_1D,
  COLONIO_DEBUG_EVENT_REQUIRED_2D,
  COLONIO_DEBUG_EVENT_KNOWN_1D,
  COLONIO_DEBUG_EVENT_KNOWN_2D
} COLONIO_DEBUG_EVENT;

/* ATTENTION: Use same value with another languages. */
typedef enum COLONIO_MAP_FAILURE_REASON {
  COLONIO_MAP_FAILURE_REASON_NONE,
  COLONIO_MAP_FAILURE_REASON_SYSTEM_ERROR,
  COLONIO_MAP_FAILURE_REASON_NOT_EXIST_KEY,
  /* COLONIO_MAP_FAILURE_REASON_EXIST_KEY, */
  COLONIO_MAP_FAILURE_REASON_CHANGED_PROPOSER
} COLONIO_MAP_FAILURE_REASON;

/* ATTENTION: Use same value with another languages. */
typedef enum COLONIO_PUBSUB_2D_FAILURE_REASON {
  COLONIO_PUBSUB_2D_FAILURE_REASON_NONE,
  COLONIO_PUBSUB_2D_FAILURE_REASON_SYSTEM_ERROR,
  COLONIO_PUBSUB_2D_FAILURE_REASON_NOONE_RECV
} COLONIO_PUBSUB_2D_FAILURE_REASON;

#define COLONIO_MAP_OPTION_ERROR_WITHOUT_EXIST 0x1

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
      unsigned int len;
      char* str;
    } string_v;
  } value;
} colonio_value_t;

COLONIO_PUBLIC void colonio_init(colonio_t* colonio, void (*on_require_invoke)(colonio_t*, unsigned int));
COLONIO_PUBLIC void colonio_connect(
    colonio_t* colonio, const char* url, const char* token, void (*on_success)(colonio_t*),
    void (*on_failure)(colonio_t*));
COLONIO_PUBLIC void colonio_disconnect(colonio_t* colonio);
COLONIO_PUBLIC colonio_map_t colonio_access_map(colonio_t* colonio, const char* name);
COLONIO_PUBLIC colonio_pubsub_2d_t colonio_access_pubsub_2d(colonio_t* colonio, const char* name);
COLONIO_PUBLIC void colonio_get_local_nid(colonio_t* colonio, char* dest);
COLONIO_PUBLIC void colonio_set_position(colonio_t* colonio, double x, double y);

COLONIO_PUBLIC void colonio_set_on_output_log(
    colonio_t* colonio, void (*func)(colonio_t*, COLONIO_LOG_LEVEL, const char*, unsigned int));
COLONIO_PUBLIC void colonio_set_on_debug_event(
    colonio_t* colonio, void (*func)(colonio_t*, COLONIO_DEBUG_EVENT, const char*, unsigned int));
COLONIO_PUBLIC unsigned int colonio_invoke(colonio_t* colonio);

COLONIO_PUBLIC void colonio_value_init(colonio_value_t* value);
COLONIO_PUBLIC COLONIO_VALUE_TYPE colonio_value_get_type(const colonio_value_t* value);
COLONIO_PUBLIC bool colonio_value_get_bool(colonio_value_t* value);
COLONIO_PUBLIC int64_t colonio_value_get_int(colonio_value_t* value);
COLONIO_PUBLIC double colonio_value_get_double(colonio_value_t* value);
COLONIO_PUBLIC unsigned int colonio_value_get_string_len(colonio_value_t* value);
COLONIO_PUBLIC void colonio_value_get_string(colonio_value_t* value, char* dest);
COLONIO_PUBLIC void colonio_value_set_bool(colonio_value_t* value, bool v);
COLONIO_PUBLIC void colonio_value_set_int(colonio_value_t* value, int64_t v);
COLONIO_PUBLIC void colonio_value_set_double(colonio_value_t* value, double v);
COLONIO_PUBLIC void colonio_value_set_string(colonio_value_t* value, const char* v, unsigned int len);
COLONIO_PUBLIC void colonio_value_free(colonio_value_t* value);

COLONIO_PUBLIC void colonio_map_get(
    colonio_map_t* map, const colonio_value_t* key, void* ptr,
    void (*on_success)(colonio_map_t* map, void* ptr, const colonio_value_t* v),
    void (*on_failure)(colonio_map_t* map, void* ptr, COLONIO_MAP_FAILURE_REASON reason));
COLONIO_PUBLIC void colonio_map_set(
    colonio_map_t* map, const colonio_value_t* key, const colonio_value_t* value, void* ptr,
    void (*on_success)(colonio_map_t* map, void* ptr),
    void (*on_failure)(colonio_map_t* map, void* ptr, COLONIO_MAP_FAILURE_REASON reason), int opt);

COLONIO_PUBLIC void colonio_pubsub_2d_publish(
    colonio_pubsub_2d_t* pubsub_2d, const char* name, unsigned int name_siz, double x, double y, double r,
    const colonio_value_t* value, void* ptr, void (*on_success)(colonio_pubsub_2d_t* pubsub_2d, void* ptr),
    void (*on_failure)(colonio_pubsub_2d_t* pubsub_2d, void* ptr, COLONIO_PUBSUB_2D_FAILURE_REASON reason));
COLONIO_PUBLIC void colonio_pubsub_2d_on(
    colonio_pubsub_2d_t* pubsub_2d, const char* name, unsigned int name_siz, void* ptr,
    void (*subscriber)(colonio_pubsub_2d_t* pubsub_2d, void* ptr, const colonio_value_t* value));
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
