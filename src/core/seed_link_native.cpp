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

#include "seed_link_native.hpp"

#include "logger.hpp"

namespace colonio {

SeedLinkNative::Init SeedLinkNative::initializer;

SeedLinkNative::Init::Init() {
  curl_global_init(CURL_GLOBAL_ALL);
}

SeedLinkNative::Init::~Init() {
  curl_global_cleanup();
}

SeedLinkNative::Task::Task(
    CURL* h, const std::string& u, const std::string& r, std::function<void(int, const std::string&)> c) :
    post(false), handle(h), url(u), request(r), cb(c) {
}

SeedLinkNative::SeedLinkNative(SeedLinkParam& param) :
    SeedLink(param), terminate(false), multi_handle(nullptr), headers(nullptr) {
  th      = std::make_unique<std::thread>(std::bind(&SeedLinkNative::sub_routine, this));
  headers = curl_slist_append(headers, "Content-Type: application/octet-stream");
}

SeedLinkNative::~SeedLinkNative() {
  {
    std::lock_guard<std::mutex> lock(mtx);
    terminate = true;
  }
  // curl_multi_wakeup(multi_handle);
  cv.notify_all();
  th->join();
  curl_slist_free_all(headers);
}

void SeedLinkNative::post(
    const std::string& path, const std::string& data, std::function<void(int, const std::string&)>&& cb_response) {
  CURL* handle               = curl_easy_init();
  std::unique_ptr<Task> task = std::make_unique<Task>(handle, URL + path, data, cb_response);

  // HTTP/2
  curl_easy_setopt(handle, CURLOPT_HTTP_VERSION, CURL_HTTP_VERSION_2_0);

  // URL
  curl_easy_setopt(handle, CURLOPT_URL, task->url.c_str());

  // headers
  curl_easy_setopt(handle, CURLOPT_HTTPHEADER, headers);

  // callback
  curl_easy_setopt(handle, CURLOPT_WRITEDATA, task.get());
  curl_easy_setopt(handle, CURLOPT_WRITEFUNCTION, SeedLinkNative::curl_cb);

  // disable verification
  if (DISABLE_VERIFICATION) {
    curl_easy_setopt(handle, CURLOPT_SSL_VERIFYPEER, 0);
    curl_easy_setopt(handle, CURLOPT_SSL_VERIFYHOST, 0);
  }

#if (CURLPIPE_MULTIPLEX > 0)
  curl_easy_setopt(handle, CURLOPT_PIPEWAIT, 1);
#endif

  // post data
  curl_easy_setopt(handle, CURLOPT_POSTFIELDS, task->request.c_str());
  curl_easy_setopt(handle, CURLOPT_POSTFIELDSIZE, -1L);

  std::lock_guard<std::mutex> lock(mtx);
  tasks.insert(std::make_pair(handle, std::move(task)));
  // curl_multi_wakeup(multi_handle);
  cv.notify_all();
}

void SeedLinkNative::sub_routine() {
  multi_handle = curl_multi_init();
  curl_multi_setopt(multi_handle, CURLMOPT_PIPELINING, CURLPIPE_MULTIPLEX);

  while (true) {
    {
      std::unique_lock<std::mutex> lock(mtx);
      if (tasks.size() == 0) {
        cv.wait(lock, [this]() {
          return tasks.size() != 0 || terminate;
        });
      }

      // exit if the terminate flag was on
      if (terminate) {
        break;
      }
    }

    int still_running = 0;
    CURLMcode mc      = curl_multi_perform(multi_handle, &still_running);
    if (mc != CURLM_OK) {
      log_warn(curl_multi_strerror(mc));
      continue;
    }

    if (still_running != 0) {
      /* curl_multi_poll looks not working. instead of it, use sleep_for
      mc = curl_multi_poll(multi_handle, nullptr, 0, 1000, nullptr);
      if (mc != CURLM_OK) {
        log_warn(curl_multi_strerror(mc));
        continue;
      }
      */
      std::this_thread::sleep_for(std::chrono::milliseconds(500));
    }

    {
      std::lock_guard<std::mutex> lock(mtx);

      for (const auto& it : tasks) {
        if (!it.second->post) {
          curl_multi_add_handle(multi_handle, it.first);
          it.second->post = true;
        }
      }
    }

    // detect to finish receiving response
    while (true) {
      int msgq     = 0;
      CURLMsg* msg = curl_multi_info_read(multi_handle, &msgq);
      // break if the info is empty
      if (msgq == 0 && msg == nullptr) {
        break;
      }

      // skip if transfer is not done
      if (msg->msg != CURLMSG_DONE) {
        continue;
      }

      CURL* handler = msg->easy_handle;
      std::unique_ptr<Task> task;
      {
        std::lock_guard<std::mutex> lock(mtx);
        auto it = tasks.find(handler);
        assert(it != tasks.end());
        task = std::move(it->second);
        tasks.erase(it);
      }

      if (msg->data.result == 0) {
        long http_code = 0;
        curl_easy_getinfo(handler, CURLINFO_RESPONSE_CODE, &http_code);
        task->cb(http_code, task->response);

      } else {
        log_warn(curl_easy_strerror(msg->data.result));
        task->cb(0, "");
      }

      curl_multi_remove_handle(multi_handle, handler);
      curl_easy_cleanup(handler);
    }
  }

  curl_multi_cleanup(multi_handle);
}

size_t SeedLinkNative::curl_cb(void* contents, size_t size, size_t nmemb, void* userp) {
  Task* task       = reinterpret_cast<Task*>(userp);
  size_t real_size = size * nmemb;

  task->response += std::string(reinterpret_cast<const char*>(contents), real_size);

  return real_size;
}

}  // namespace colonio
