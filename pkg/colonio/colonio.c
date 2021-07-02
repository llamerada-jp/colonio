/*
 * Copyright 2020 Yuji Ito <llamerada.jp@gmail.com>
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
const unsigned int cgo_colonio_nid_length                    = COLONIO_NID_LENGTH;
const unsigned int cgo_colonio_colonio_explicit_event_thread = COLONIO_COLONIO_EXPLICIT_EVENT_THREAD;

// colonio
colonio_error_t *cgo_colonio_connect(colonio_t *colonio, _GoString_ url, _GoString_ token) {
  return colonio_connect(colonio, _GoStringPtr(url), _GoStringLen(url), _GoStringPtr(token), _GoStringLen(token));
}

colonio_map_t cgo_colonio_access_map(colonio_t *colonio, _GoString_ name) {
  return colonio_access_map(colonio, _GoStringPtr(name), _GoStringLen(name));
}

colonio_pubsub_2d_t cgo_colonio_access_pubsub_2d(colonio_t *colonio, _GoString_ name) {
  return colonio_access_pubsub_2d(colonio, _GoStringPtr(name), _GoStringLen(name));
}

// value
void cgo_colonio_value_set_string(colonio_value_t *value, _GoString_ s) {
  colonio_value_set_string(value, _GoStringPtr(s), _GoStringLen(s));
}

// pubsub
colonio_error_t *cgo_colonio_pubsub_2d_publish(
    colonio_pubsub_2d_t *pubsub_2d, _GoString_ name, double x, double y, double r, const colonio_value_t *value,
    uint32_t opt) {
  colonio_pubsub_2d_publish(pubsub_2d, _GoStringPtr(name), _GoStringLen(name), x, y, r, value, opt);
}

void cgo_cb_colonio_pubsub_2d_on(colonio_pubsub_2d_t *pubsub_2d, void *ptr, const colonio_value_t *val) {
  cgoCbPubsub2DOn(pubsub_2d, (void *)ptr, (void *)val);
}

void cgo_colonio_pubsub_2d_on(colonio_pubsub_2d_t *pubsub_2d, _GoString_ name, void *ptr) {
  colonio_pubsub_2d_on(pubsub_2d, _GoStringPtr(name), _GoStringLen(name), ptr, cgo_cb_colonio_pubsub_2d_on);
}

void cgo_colonio_pubsub_2d_off(colonio_pubsub_2d_t *pubsub_2d, _GoString_ name) {
  colonio_pubsub_2d_off(pubsub_2d, _GoStringPtr(name), _GoStringLen(name));
}
