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
#pragma once

#include <cstdint>

namespace colonio {

/**
 * @brief ErrorCode is assigned each error reason and is used with Error and Exception.
 *
 * @sa Error, Exception
 */
enum class ErrorCode : uint32_t {
  UNDEFINED,              ///< Undefined error is occured.
  SYSTEM_ERROR,           ///< An error occurred in the API, which is used inside colonio.
  OFFLINE,                ///< The node cannot perform processing because of offline.
  INCORRECT_DATA_FORMAT,  ///< Incorrect data format detected.
  CONFLICT_WITH_SETTING,  ///< The calling method or setting parameter was inconsistent with the configuration in the
                          ///< seed.
  NOT_EXIST_KEY,          ///< Tried to get a value for a key that doesn't exist.
  EXIST_KEY,              ///< Under developing.
  CHANGED_PROPOSER,       ///< Under developing.
  COLLISION_LATE,         ///< Under developing.
  NO_ONE_RECV,            ///< There was no node receiving the message.
};

/**
 * @brief The urgency of the log.
 *
 * @sa Colonio::on_output_log
 */
enum class LogLevel : uint32_t {
  INFO,   ///< Logs of normal operations.
  WARN,   ///< Logs errors that do not require program interruption.
  ERROR,  ///< Logs errors that require end of colonio.
  DEBUG,  ///< Logs for debugging, This is output only when build with debug flag.
};

}  // namespace colonio
