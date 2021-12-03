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

#include <colonio/definition.hpp>
#include <exception>
#include <string>

namespace colonio {

/**
 * @brief Error information. This is used when the asynchronous method calls a failed callback and is thrown when an
 * error occurs in a synchronous method.
 *
 *
 * @sa ErrorCode
 */
class Error : public std::exception {
 public:
  /// True if the error is fatal and the process of colonio can not continue.
  const bool fatal;
  /// Code to indicate the cause of the error.
  const ErrorCode code;
  /// A detailed message string for display or bug report.
  const std::string message;
  /// The line number where the exception was thrown (for debug).
  const unsigned long line;
  /// The file name where the exception was thrown (for debug).
  const std::string file;

  /**
   * @brief Construct a new Error object.
   *
   * @param fatal True if the error is fatal and the process of colonio can not continue.
   * @param code Code to indicate the cause of the error.
   * @param message A detailed message string for display or bug report.
   * @param line The line number where the exception was thrown.
   * @param file The file name where the exception was thrown.
   */
  Error(bool fatal, ErrorCode code, const std::string& message, int line, const std::string& file);

  /**
   * @brief Override the standard method to output message.
   *
   * @return const char* The message is returned as it is.
   */
  const char* what() const noexcept override;
};
}  // namespace colonio
