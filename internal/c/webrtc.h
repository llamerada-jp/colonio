/*
 * Copyright 2017- Yuji Ito <llamerada.jp@gmail.com>
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
#pragma once

#ifdef __cplusplus
extern "C" {
#endif

void webrtc_config_init();
unsigned int webrtc_config_new(const char* ice, int ice_len);
void webrtc_config_destruct(unsigned int id);

void webrtc_link_get_error_message(const char** message, int* message_len);
void webrtc_link_init(
    void (*update_state_cb)(unsigned int, int), void (*update_ice_cb)(unsigned int, const void*, int),
    void (*receive_data_cb)(unsigned int, const void*, int), void (*error_cb)(unsigned int, const char*, int));
unsigned int webrtc_link_new(unsigned int config_id, int create_data_channel);
int webrtc_link_disconnect(unsigned int id);
int webrtc_link_get_local_sdp(unsigned int id, const char** sdp, int* sdp_len);
int webrtc_link_set_remote_sdp(unsigned int id, const char* sdp, int sdp_len);
int webrtc_link_update_ice(unsigned int id, const char* ice, int ice_len);
int webrtc_link_send(unsigned int id, const void* data, int data_len);

#ifdef __cplusplus
}
#endif