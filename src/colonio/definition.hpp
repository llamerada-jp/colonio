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
#include <string>

namespace colonio {

/**
 * @brief ErrorCode is assigned each error reason and is used with Error and Exception.
 *
 * @sa Error, Exception
 */
enum class ErrorCode : uint32_t {
  UNDEFINED,              ///< Undefined error is occurred.
  SYSTEM_ERROR,           ///< An error occurred in the API, which is used inside colonio.
  OFFLINE,                ///< The node cannot perform processing because of offline.
  INCORRECT_DATA_FORMAT,  ///< Incorrect data format detected.
  CONFLICT_WITH_SETTING,  ///< The calling method or setting parameter was inconsistent with the configuration in the
                          ///< seed.
  NOT_EXIST_KEY,          ///< Tried to get a value for a key that doesn't exist.
  EXIST_KEY,              ///< An error occurs when overwriting the value for an existing key.
  CHANGED_PROPOSER,       ///< Under developing.
  COLLISION_LATE,         ///< Under developing.
  NO_ONE_RECV,            ///< There was no node receiving the message.
};

/**
 * @brief Defines of string to be the keys for the log JSON.
 *
 * @sa Colonio::on_output_log
 * @sa LogLevel
 */
namespace LogJSONKey {
static const std::string FILE("file");        ///< Log output file name.
static const std::string LEVEL("level");      ///< The urgency of the log.
static const std::string LINE("line");        ///< Log output line number.
static const std::string MESSAGE("message");  ///< Readable log messages.
static const std::string PARAM("param");      ///< The parameters that attached for the log.
static const std::string TIME("time");        ///< Log output time.
}  // namespace LogJSONKey

/**
 * @brief The urgency of the log.
 *
 * @sa Colonio::on_output_log
 */
namespace LogLevel {
static const std::string INFO("info");    ///< Logs of normal operations.
static const std::string WARN("warn");    ///< Logs errors that do not require program interruption.
static const std::string ERROR("error");  ///< Logs errors that require end of colonio.
static const std::string DEBUG("debug");  ///< Logs for debugging, This is output only when build with debug flag.
}  // namespace LogLevel

}  // namespace colonio
