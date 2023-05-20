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
#pragma once

#include <curl/curl.h>

#include <condition_variable>
#include <map>
#include <memory>
#include <mutex>
#include <thread>

#include "seed_link.hpp"

#ifndef CURLPIPE_MULTIPLEX
#  define CURLPIPE_MULTIPLEX 0
#endif

namespace colonio {
class SeedLinkNative : public SeedLink {
 public:
  explicit SeedLinkNative(SeedLinkParam& param);
  virtual ~SeedLinkNative();

  // SeedLinkBase
  void post(
      const std::string& path, const std::string& data,
      std::function<void(int, const std::string&)>&& cb_response) override;

 private:
  static class Init {
   public:
    Init();
    virtual ~Init();
  } initializer;

  class Task {
   public:
    bool post;
    CURL* handle;
    const std::string url;
    const std::string request;
    std::string response;
    std::function<void(int, const std::string&)> cb;

    Task(CURL* h, const std::string& u, const std::string& r, std::function<void(int, const std::string&)> c);
    virtual ~Task();
  };

  bool terminate;
  std::unique_ptr<std::thread> th;
  std::mutex mtx;
  std::condition_variable cv;
  std::map<CURL*, std::unique_ptr<Task>> tasks;
  CURLM* multi_handle;
  // common header to set content-type
  curl_slist* headers;

  static size_t curl_cb(void* contents, size_t size, size_t nmemb, void* userp);

  void sub_routine();
};
}  // namespace colonio
