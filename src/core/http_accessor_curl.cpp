/*
 * Copyright 2017-2019 Yuji Ito <llamerada.jp@gmail.com>
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
#include <functional>
#include <mutex>
#include <string>

#include "http_accessor_curl.hpp"

namespace colonio {

static size_t curl_write_callback(void *contents, size_t size, size_t nmemb, void *userp) {
  std::string& content = *reinterpret_cast<std::string*>(userp);
  content.append(reinterpret_cast<char*>(contents), size * nmemb);
  return size * nmemb;
}

HttpAccessorCurl::HttpAccessorCurl() {
  curl_global_init(CURL_GLOBAL_ALL);

  curl = curl_easy_init();
  if (curl == nullptr) {
    // TODO(llamerada.jp@gmail.com): error on connect to the server.
  }

  curl_easy_setopt(curl, CURLOPT_COOKIEFILE, "");
  curl_easy_setopt(curl, CURLOPT_COOKIESESSION, 1);
}

HttpAccessorCurl::~HttpAccessorCurl() {
  if (curl != nullptr) {
    curl_easy_cleanup(curl);
  }
  curl_global_cleanup();
}

void HttpAccessorCurl::post(const std::string& url, const std::string& payload,
                            const std::string& content_type, HttpAccessorDelegate* delegate) {
  assert(curl != nullptr);
  assert(delegate != nullptr);

  // Set post parameters.
  curl_easy_setopt(curl, CURLOPT_URL, url.c_str());
  curl_easy_setopt(curl, CURLOPT_POST, 1);
  curl_easy_setopt(curl, CURLOPT_POSTFIELDS, payload.c_str());

  // Set header for http protocol.
  curl_slist *headers = nullptr;
  std::string type_str = "Content-Type: " + content_type;
  headers = curl_slist_append(headers, type_str.c_str());  
  curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);

  // To get response content.
  std::string content;
  CURLcode res;
  long http_code = 0;
  {
    std::lock_guard<std::mutex> guard(mutex);

    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, curl_write_callback);
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &content);
    res = curl_easy_perform(curl);

    // Free header data.
    curl_slist_free_all(headers);

    // Get return code.
    curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &http_code);
  }

  if (res == CURLE_OK) {
    delegate->http_accessor_on_response(*this, http_code, content);

  } else {
    if (http_code == 0) {
      delegate->http_accessor_on_error(*this, curl_easy_strerror(res), payload);

    } else {
      delegate->http_accessor_on_response(*this, http_code, content);
    }
  }
}
}  // namespace colonio
