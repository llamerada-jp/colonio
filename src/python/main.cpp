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
#include <pybind11/functional.h>
#include <pybind11/pybind11.h>

#include "colonio/colonio_libuv.hpp"
#include "colonio/map.hpp"

namespace py = pybind11;

class PythonMap {
 public:
  colonio::Map& map;

  PythonMap(colonio::Map& map_) : map(map_) {
  }

  static colonio::Value convertValue(const py::object& src) {
    if (py::bool_::check_(src)) {
      return colonio::Value(src.cast<bool>());

    } else if (py::int_::check_(src)) {
      return colonio::Value(static_cast<int64_t>(src.cast<int>()));

    } else if (py::float_::check_(src)) {
      return colonio::Value(src.cast<double>());

    } else if (py::str::check_(src)) {
      return colonio::Value(src.cast<std::string>());

    } else {
      assert(false);
      return colonio::Value();
    }
  }

  static std::unique_ptr<py::object> convertValue(const colonio::Value& src) {
    switch (src.get_type()) {
      case colonio::Value::NULL_T:
        return std::unique_ptr<py::object>(new py::none());

      case colonio::Value::BOOL_T:
        return std::unique_ptr<py::object>(new py::bool_(src.get<bool>()));

      case colonio::Value::INT_T:
        return std::unique_ptr<py::object>(new py::int_(src.get<int64_t>()));

      case colonio::Value::DOUBLE_T:
        return std::unique_ptr<py::object>(new py::float_(src.get<double>()));

      case colonio::Value::STRING_T:
        return std::unique_ptr<py::object>(new py::str(src.get<std::string>()));
    }
  }

  void get(
      const py::object key, const std::function<void(py::object&)>& on_success,
      const std::function<void(colonio::Exception::Code)>& on_failure) {
    colonio::Value key_value = convertValue(key);
    map.get(
        key_value,
        [on_success](const colonio::Value& value) {
          std::unique_ptr<py::object> val = std::move(convertValue(value));
          on_success(*val);
        },
        on_failure);
  }

  void set(
      const py::object key, const py::object val, const std::function<void()>& on_success,
      const std::function<void(colonio::Exception::Code)>& on_failure) {
    colonio::Value key_value = convertValue(key);
    colonio::Value val_value = convertValue(val);
    map.set(key_value, val_value, on_success, on_failure);
  }
};

class PythonColonio : public colonio_helper::ColonioLibuv {
 public:
  enum RUN_MODE {
    DEFAULT,
    ONCE,
    NOWAIT,
  };

  PythonColonio() : ColonioLibuv(nullptr) {
  }

  void _connect(
      const std::string& url, const std::string& token, std::function<void(PythonColonio&)> on_success,
      std::function<void(PythonColonio&)> on_failure) {
    on_connect_success = on_success;
    on_connect_failure = on_failure;
    connect(
        url, token, [this](colonio::Colonio&) { on_connect_success(*this); },
        [this](colonio::Colonio&) { on_connect_failure(*this); });
  }

  void _run(RUN_MODE mode) {
    switch (mode) {
      case DEFAULT:
        run(UV_RUN_DEFAULT);
        break;
      case ONCE:
        run(UV_RUN_ONCE);
        break;
      case NOWAIT:
        run(UV_RUN_NOWAIT);
        break;
    }
  }

 private:
  std::function<void(PythonColonio&)> on_connect_success;
  std::function<void(PythonColonio&)> on_connect_failure;
};

PYBIND11_MODULE(colonio, m) {
  m.doc() = "colonio module";
  // Map
  py::class_<PythonMap> Map(m, "Map");
  Map.def("get", &PythonMap::get).def("set", &PythonMap::set);

  py::enum_<colonio::Exception::Code>(Map, "FailureReason")
      .value("NONE", colonio::Exception::Code::NONE)
      .value("SYSTEM_ERROR", colonio::Exception::Code::SYSTEM_ERROR)
      .value("NOT_EXIST_KEY", colonio::Exception::Code::NOT_EXIST_KEY)
      .value("CHANGED_PROPOSER", colonio::Exception::Code::CHANGED_PROPOSER)
      .export_values();

  // Colonio
  py::class_<PythonColonio> Colonio(m, "Colonio");

  Colonio.def(py::init<>())
      .def("connect", &PythonColonio::_connect)
      .def("run", &PythonColonio::_run)
      .def("disconnect", &PythonColonio::disconnect)
      .def("getLocalNid", &PythonColonio::get_local_nid);

  py::enum_<PythonColonio::RUN_MODE>(Colonio, "RunMode")
      .value("DEFAULT", PythonColonio::RUN_MODE::DEFAULT)
      .value("ONCE", PythonColonio::RUN_MODE::ONCE)
      .value("NOWAIT", PythonColonio::RUN_MODE::NOWAIT)
      .export_values();
}
