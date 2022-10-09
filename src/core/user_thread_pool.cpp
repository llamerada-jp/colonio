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

#include "user_thread_pool.hpp"

#include <cassert>
namespace colonio {

UserThreadPool::Task::Task(std::function<void()>& f) : func(f) {
}

UserThreadPool::Thread::Thread(std::function<void()> f) : th(f), dead(false) {
}

UserThreadPool::UserThreadPool(Logger& l, unsigned int max_threads, unsigned int bored_limit) :
    MAX_THREADS(max_threads), BORED_LIMIT(bored_limit), logger(l), fin(false) {
}

UserThreadPool::~UserThreadPool() {
  std::unique_lock<std::mutex> lock(mtx);
  fin = true;
  for (auto& it : threads) {
    Thread& th = *it.second;
    if (!th.dead) {
      th.cv.notify_all();
    }
  }

  threads_cv.wait(lock, [&, this] {
    house_keep();
    return threads.empty();
  });
}

void UserThreadPool::push(std::function<void()>&& func) {
  std::unique_lock<std::mutex> lock(mtx);

  assert(!fin);

retry:
  if (free_threads.size() == 0) {
    // create a new thread if there is room
    if (threads.size() < MAX_THREADS) {
      std::unique_ptr<Thread> th = std::make_unique<Thread>(std::bind(&UserThreadPool::start_on_thread, this));
      threads.insert(std::make_pair(th->th.get_id(), std::move(th)));
      threads_cv.notify_all();
    }

    // wait to get a free thread
    free_threads_cv.wait(lock, [this] {
      return free_threads.size() > 0;
    });
  }

  // pass a new task for the thread that does not have any task
  std::thread::id free_tid = free_threads.back();
  free_threads.pop_back();
  Thread* th = threads.at(free_tid).get();
  assert(!th->task);
  if (th->dead) {
    house_keep();
    goto retry;
  }
  th->task = std::make_unique<Task>(func);
  th->cv.notify_all();

  house_keep();
}

void UserThreadPool::start_on_thread() {
  std::thread::id tt_id = std::this_thread::get_id();
  {
    std::unique_lock<std::mutex> lock(mtx);
    threads_cv.wait(lock, [this, tt_id] {
      return threads.find(tt_id) != threads.end();
    });
  }
  Thread& tt = *threads.at(tt_id);

  while (true) {
    std::unique_lock<std::mutex> lock(mtx);

    // wait until a new task will come
    if (!tt.task) {
      free_threads.push_back(tt_id);
      free_threads_cv.notify_all();
      if (!tt.cv.wait_for(lock, std::chrono::milliseconds(BORED_LIMIT), [this, &tt] {
            return fin || static_cast<bool>(tt.task);
          })) {
        // finish this thread because of too bored
        assert(!tt.task);
        tt.dead = true;
        threads_cv.notify_all();
        return;
      }
    }

    // cancel task if catch finish signal
    if (fin) {
      tt.task.reset();
      tt.dead = true;
      threads_cv.notify_all();
      return;
    }

    lock.unlock();
    tt.task->func();
    // TODO: deal with errors and exceptions
    tt.task.reset();
  }
}

void UserThreadPool::house_keep() {
  // the mutex locked in `push` func
  // std::lock_guard<std::mutex> lock(mtx);

  auto th = threads.begin();
  while (th != threads.end()) {
    if (th->first != std::this_thread::get_id() && th->second->dead) {
      assert(!th->second->task);
      th->second->th.join();
      th = threads.erase(th);
    } else {
      th++;
    }
  }

  auto tid = free_threads.begin();
  while (tid != free_threads.end()) {
    if (threads.find(*tid) == threads.end()) {
      tid = free_threads.erase(tid);
    } else {
      tid++;
    }
  }
}
}  // namespace colonio