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
#include <pybind11/functional.h>
#include <pybind11/pybind11.h>

#include <map>
#include <memory>

#include "colonio/colonio.hpp"
#include "colonio/map.hpp"

namespace py = pybind11;

class PythonMap {
 public:
  colonio::Map& map;

  explicit PythonMap(colonio::Map& map_) : map(map_) {
  }

  static colonio::Value convertValue(const py::object& src) {
    if (py::isinstance<py::bool_>(src)) {
      return colonio::Value(src.cast<bool>());

    } else if (py::isinstance<py::int_>(src)) {
      return colonio::Value(static_cast<int64_t>(src.cast<int>()));

    } else if (py::isinstance<py::float_>(src)) {
      return colonio::Value(src.cast<double>());

    } else if (py::isinstance<py::str>(src)) {
      return colonio::Value(src.cast<std::string>());

    } else {
      assert(false);
      return colonio::Value();
    }
  }

  static std::unique_ptr<py::object> convertValue(const colonio::Value& src) {
    switch (src.get_type()) {
      case colonio::Value::BOOL_T:
        return std::unique_ptr<py::object>(new py::bool_(src.get<bool>()));

      case colonio::Value::INT_T:
        return std::unique_ptr<py::object>(new py::int_(src.get<int64_t>()));

      case colonio::Value::DOUBLE_T:
        return std::unique_ptr<py::object>(new py::float_(src.get<double>()));

      case colonio::Value::STRING_T:
        return std::unique_ptr<py::object>(new py::str(src.get<std::string>()));

      default:
        assert(src.get_type() == colonio::Value::NULL_T);
        return std::unique_ptr<py::object>(new py::none());
    }
  }

  void get(
      const py::object key, const std::function<void(PythonMap&, py::object&)> on_success,
      const std::function<void(PythonMap&, colonio::Error)> on_failure) {
    colonio::Value key_value = convertValue(key);
    map.get(
        key_value,
        [this, on_success](colonio::Map&, const colonio::Value& value) {
          std::unique_ptr<py::object> val = std::move(convertValue(value));
          on_success(*this, *val);
        },
        [this, on_failure](colonio::Map&, const colonio::Error& err) {
          on_failure(*this, err);
        });
  }

  void set(
      const py::object key, const py::object val, unsigned int opt, const std::function<void(PythonMap&)> on_success,
      const std::function<void(PythonMap&, colonio::Error)> on_failure) {
    colonio::Value key_value = convertValue(key);
    colonio::Value val_value = convertValue(val);
    map.set(
        key_value, val_value, static_cast<uint32_t>(opt),
        [this, on_success](colonio::Map&) {
          on_success(*this);
        },
        [this, on_failure](colonio::Map&, const colonio::Error& err) {
          on_failure(*this, err);
        });
  }
};

class PythonColonio {
 public:
  PythonColonio() : c(std::shared_ptr<C>()) {
    c->instance.reset(colonio::Colonio::new_instance());
  }

  PythonColonio(std::function<void(const std::string&)> log_receiver, uint32_t opt) : c(std::shared_ptr<C>()) {
    c->instance.reset(colonio::Colonio::new_instance(
        [log_receiver](colonio::Colonio&, const std::string& message) {
          log_receiver(message);
        },
        opt));
  }

  PythonColonio(const PythonColonio& src) : c(src.c) {
  }

  PythonMap& access_map(const std::string& name) {
    auto it = c->map_cache.find(name);
    if (it == c->map_cache.end()) {
      it = c->map_cache.insert(std::make_pair(name, std::make_unique<PythonMap>(c->instance->access_map(name)))).first;
    }
    return *(it->second);
  }

  void connect(
      const std::string& url, const std::string& token, std::function<void(PythonColonio&)> on_success,
      std::function<void(PythonColonio&, colonio::Error)> on_failure) {
    c->instance->connect(
        url, token,
        [this, on_success](colonio::Colonio&) {
          on_success(*this);
        },
        [this, on_failure](colonio::Colonio&, const colonio::Error& err) {
          on_failure(*this, err);
        });
  }

  void disconnect() {
    c->instance->disconnect();
  }

  std::string get_local_nid() {
    return c->instance->get_local_nid();
  }

 private:
  struct C {
    std::unique_ptr<colonio::Colonio> instance;
    std::map<const std::string, std::unique_ptr<PythonMap>> map_cache;
  };

  std::shared_ptr<C> c;
};

PYBIND11_MODULE(colonio, m) {
  m.doc() = "colonio module";

  // ErrorCode
  py::enum_<colonio::ErrorCode>(m, "ErrorCode")
      .value("UNDEFINED", colonio::ErrorCode::UNDEFINED)
      .value("SYSTEM_ERROR", colonio::ErrorCode::SYSTEM_ERROR)
      .value("CONNECTION_FAILD", colonio::ErrorCode::CONNECTION_FAILD)
      .value("OFFLINE", colonio::ErrorCode::OFFLINE)
      .value("INCORRECT_DATA_FORMAT", colonio::ErrorCode::INCORRECT_DATA_FORMAT)
      .value("CONFLICT_WITH_SETTING", colonio::ErrorCode::CONFLICT_WITH_SETTING)
      .value("NOT_EXIST_KEY", colonio::ErrorCode::NOT_EXIST_KEY)
      .value("EXIST_KEY", colonio::ErrorCode::EXIST_KEY)
      .value("CHANGED_PROPOSER", colonio::ErrorCode::CHANGED_PROPOSER)
      .value("COLLISION_LATE", colonio::ErrorCode::COLLISION_LATE)
      .value("NO_ONE_RECV", colonio::ErrorCode::NO_ONE_RECV)
      .export_values();

  // Error
  py::class_<colonio::Error> Error(m, "Error");
  Error.def(py::init<colonio::ErrorCode, const std::string&>())
      .def_readonly("code", &colonio::Error::code)
      .def_readonly("message", &colonio::Error::message);

  // Map
  py::class_<PythonMap> Map(m, "Map");
  Map.def_readonly_static("ERROR_WITHOUT_EXIST", &colonio::Map::ERROR_WITHOUT_EXIST)
      .def_readonly_static("ERROR_WITH_EXIST", &colonio::Map::ERROR_WITH_EXIST)
      .def_readonly_static("TRY_LOCK", &colonio::Map::TRY_LOCK)
      .def("get", &PythonMap::get)
      .def("set", &PythonMap::set);

  // Colonio
  py::class_<PythonColonio> Colonio(m, "Colonio");
  Colonio.def(py::init<>())
      .def_readonly_static("EXPLICIT_EVENT_THREAD", &colonio::Colonio::EXPLICIT_EVENT_THREAD)
      .def_readonly_static("EXPLICIT_CONTROLLER_THREAD", &colonio::Colonio::EXPLICIT_CONTROLLER_THREAD)
      .def("accessMap", &PythonColonio::access_map)
      .def("connect", &PythonColonio::connect)
      .def("disconnect", &PythonColonio::disconnect)
      .def("getLocalNid", &PythonColonio::get_local_nid);
}
