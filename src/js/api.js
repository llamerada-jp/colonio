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

mergeInto(LibraryManager.library, {
  scheduler_release: function (p1) { schedulerRelease(p1); },
  scheduler_request_next_routine: function (p1, p2) { schedulerRequestNextRoutine(p1, p2); },

  seed_link_post: function (p1, p2, p3, p4, p5) { seedLinkPost(p1, p2, p3, p4, p5); },

  utils_get_random_seed: function () { return utilsGetRandomSeed(); },

  webrtc_context_initialize: function () { webrtcContextInitialize(); },
  webrtc_context_add_ice_server: function (p1, p2) { webrtcContextAddIceServer(p1, p2); },

  webrtc_link_initialize: function (p1, p2) { webrtcLinkInitialize(p1, p2); },
  webrtc_link_finalize: function (p1) { webrtcLinkFinalize(p1); },
  webrtc_link_disconnect: function (p1) { webrtcLinkDisconnect(p1); },
  webrtc_link_get_local_sdp: function (p1, p2) { webrtcLinkGetLocalSdp(p1, p2); },
  webrtc_link_send: function (p1, p2, p3) { webrtcLinkSend(p1, p2, p3); },
  webrtc_link_set_remote_sdp: function (p1, p2, p3, p4) { webrtcLinkSetRemoteSdp(p1, p2, p3, p4); },
  webrtc_link_update_ice: function (p1, p2, p3) { webrtcLinkUpdateIce(p1, p2, p3); }
});
