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
#include "_cgo_export.h"

// copy functions from _obj/colonio/cgo2.c
static size_t _GoStringLen(_GoString_ s) {
  return (size_t)s.n;
}

static const char *_GoStringPtr(_GoString_ s) {
  return s.p;
}

// export constant value for golang
const unsigned int cgo_colonio_nid_length = COLONIO_NID_LENGTH;

// colonio
void cgo_cb_colonio_logger(colonio_t *colonio, const char *message, unsigned int len) {
  cgoCbColonioLogger(colonio, (void *)message, (int)len);
}

colonio_error_t *cgo_colonio_init(colonio_t *colonio) {
  return colonio_init(colonio, cgo_cb_colonio_logger, COLONIO_COLONIO_EXPLICIT_EVENT_THREAD);
}

colonio_error_t *cgo_colonio_connect(colonio_t *colonio, _GoString_ url, _GoString_ token) {
  return colonio_connect(colonio, _GoStringPtr(url), _GoStringLen(url), _GoStringPtr(token), _GoStringLen(token));
}

int cgo_colonio_is_connected(colonio_t* colonio) {
  return colonio_is_connected(colonio) ? 1 : 0;
}

colonio_map_t cgo_colonio_access_map(colonio_t *colonio, _GoString_ name) {
  return colonio_access_map(colonio, _GoStringPtr(name), _GoStringLen(name));
}

colonio_pubsub_2d_t cgo_colonio_access_pubsub_2d(colonio_t *colonio, _GoString_ name) {
  return colonio_access_pubsub_2d(colonio, _GoStringPtr(name), _GoStringLen(name));
}

colonio_error_t *cgo_colonio_call_by_nid(colonio_t *colonio, _GoString_ dst, _GoString_ name, const colonio_value_t *val, uint32_t opt, colonio_value_t* result) {
  return colonio_call_by_nid(colonio, _GoStringPtr(dst), _GoStringPtr(name), _GoStringLen(name), val, opt, result);
}

void cgo_cb_colonio_call_on(colonio_t* colonio, void* ptr, const colonio_on_call_parameter_t* parameter, colonio_value_t* result) {
  cgoCbColonioOnCall(colonio, (void *)ptr, (void*)parameter->name, (int)parameter->name_siz, (void*)parameter->value, (int)parameter->opt, (void *)result);
}

void cgo_colonio_call_on(colonio_t *colonio, _GoString_ name, uintptr_t ptr) {
  colonio_on_call(colonio, _GoStringPtr(name), _GoStringLen(name), (void *)ptr, cgo_cb_colonio_call_on);
}

void cgo_colonio_call_off(colonio_t *colonio, _GoString_ name) {
  colonio_off_call(colonio, _GoStringPtr(name), _GoStringLen(name));
}

// value
void cgo_colonio_value_set_string(colonio_value_t *value, _GoString_ s) {
  colonio_value_set_string(value, _GoStringPtr(s), _GoStringLen(s));
}

// map
void cgo_cb_colonio_map_foreach_local_value(
    colonio_map_t *map, void *ptr, const colonio_value_t *key, const colonio_value_t *val, uint32_t attr) {
  cgoColonioMapForeachLocalValue(map, (void *)ptr, (void *)key, (void *)val, attr);
}

colonio_error_t *cgo_colonio_map_foreach_local_value(colonio_map_t *map, uintptr_t ptr) {
  return colonio_map_foreach_local_value(map, (void *)ptr, cgo_cb_colonio_map_foreach_local_value);
}

// pubsub
colonio_error_t *cgo_colonio_pubsub_2d_publish(
    colonio_pubsub_2d_t *pubsub_2d, _GoString_ name, double x, double y, double r, const colonio_value_t *value,
    uint32_t opt) {
  return colonio_pubsub_2d_publish(pubsub_2d, _GoStringPtr(name), _GoStringLen(name), x, y, r, value, opt);
}

void cgo_cb_colonio_pubsub_2d_on(colonio_pubsub_2d_t *pubsub_2d, void *ptr, const colonio_value_t *val) {
  cgoCbPubsub2DOn(pubsub_2d, (void *)ptr, (void *)val);
}

void cgo_colonio_pubsub_2d_on(colonio_pubsub_2d_t *pubsub_2d, _GoString_ name, uintptr_t ptr) {
  colonio_pubsub_2d_on(pubsub_2d, _GoStringPtr(name), _GoStringLen(name), (void *)ptr, cgo_cb_colonio_pubsub_2d_on);
}

void cgo_colonio_pubsub_2d_off(colonio_pubsub_2d_t *pubsub_2d, _GoString_ name) {
  colonio_pubsub_2d_off(pubsub_2d, _GoStringPtr(name), _GoStringLen(name));
}
