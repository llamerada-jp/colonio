##
## Protocol Buffer files.
##
file(GLOB_RECURSE PROTO_FILES RELATIVE ${CMAKE_CURRENT_SOURCE_DIR} "*.proto")
foreach(PROTO_FILE in ${PROTO_FILES})
  string(REGEX REPLACE "[.]proto$" ".pb.cc" OUTPUT_SOURCE ${PROTO_FILE})
  add_custom_command(
    OUTPUT ${OUTPUT_SOURCE}
    COMMAND ${LOCAL_ENV_PATH}/bin/protoc --cpp_out=${CMAKE_CURRENT_BINARY_DIR} ${PROTO_FILE}
    COMMAND ${LOCAL_ENV_PATH}/bin/protoc --cpp_out=${CMAKE_CURRENT_SOURCE_DIR} ${PROTO_FILE}
    DEPENDS ${PROTO_FILE}
    WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
    )
endforeach()

##
## Common core files.
##
file(GLOB HEADER_FILES "*.hpp")
set(COLONIO_FILES
  core/api.pb.cc
  core/api_base.cpp
  core/api_bundler.cpp
  core/api_gate.cpp
  core/colonio_api.pb.cc
  core/command.cpp
  core/context.cpp
  core/controller.cpp
  core/convert.cpp
  core/coord_system.cpp
  core/coord_system_plane.cpp
  core/coord_system_sphere.cpp
  core/coordinate.cpp
  core/core.pb.cc
  core/error.cpp
  core/exception.cpp
  core/export_c.cpp
  core/internal_exception.cpp
  core/logger.cpp
  core/map.cpp
  core/map_api.pb.cc
  core/map_impl.cpp
  core/module_1d.cpp
  core/module_2d.cpp
  core/module_base.cpp
  core/module_bundler.cpp
  core/node_accessor.cpp
  core/node_accessor_protocol.pb.cc
  core/node_id.cpp
  core/pubsub_2d.cpp
  core/pubsub_2d_api.pb.cc
  core/pubsub_2d_impl.cpp
  core/random.cpp
  core/routing.cpp
  core/routing_protocol.pb.cc
  core/routing_1d.cpp
  core/routing_2d.cpp
  core/seed_accessor.cpp
  core/seed_accessor_protocol.pb.cc
  core/seed_link.cpp
  core/scheduler.cpp
  core/utils.cpp
  core/value.cpp
  core/value_impl.cpp
  core/colonio.cpp
  core/colonio_impl.cpp
  core/webrtc_context.cpp
  core/webrtc_link.cpp

  map_paxos/map_paxos_api.cpp
  map_paxos/map_paxos_module.cpp
  map_paxos/map_paxos_protocol.pb.cc

  pubsub_2d/pubsub_2d_api.cpp
  pubsub_2d/pubsub_2d_module.cpp
  pubsub_2d/pubsub_2d_protocol.pb.cc
  )

if(${CMAKE_SYSTEM_NAME} MATCHES "Emscripten")
  ##
  ## WebAssembly
  ##
  list(APPEND COLONIO_FILES core/md5/md5.c)
  list(APPEND COLONIO_FILES core/api_gate_wasm.cpp)
  list(APPEND COLONIO_FILES core/webrtc_context_wasm.cpp)
  list(APPEND COLONIO_FILES core/webrtc_link_wasm.cpp)
  list(APPEND COLONIO_FILES core/seed_link_websocket_wasm.cpp)

  add_executable(colonio
    js/main.cpp
    ${HEADER_FILES}
    ${COLONIO_FILES}
    )
  target_link_libraries(colonio
    ${webassembly_libs}
    )
  add_custom_target(install_colonio ALL
    COMMAND "cp" "colonio.js" "colonio.wasm" "${PROJECT_SOURCE_DIR}/local/lib/"
    DEPENDS colonio
    )

else()
  ##
  ## Native
  ##
  list(APPEND COLONIO_FILES core/api_gate_mt.cpp)
  list(APPEND COLONIO_FILES core/webrtc_context_native.cpp)
  list(APPEND COLONIO_FILES core/webrtc_link_native.cpp)
  list(APPEND COLONIO_FILES core/seed_link_websocket_native.cpp)

  # Core
  add_library(colonio STATIC
    ${HEADER_FILES}
    ${COLONIO_FILES}
    )
  install(TARGETS colonio
    RUNTIME DESTINATION ${PROJECT_SOURCE_DIR}/local/bin
    LIBRARY DESTINATION ${PROJECT_SOURCE_DIR}/local/lib
    ARCHIVE DESTINATION ${PROJECT_SOURCE_DIR}/local/lib)

  # Python
  if (WITH_PYTHON)
    find_package(pybind11 REQUIRED)
    add_library(colonio_py MODULE
      python/main.cpp
    #  ${HEADER_FILES}
    #  ${COLONIO_FILES}
      )
    target_link_libraries(colonio_py PRIVATE
      pybind11::module
      ${webrtc_libs}
      ${extra_libs}
      )
    set_target_properties(colonio_py PROPERTIES
      PREFIX "${PYTHON_MODULE_PREFIX}"
      SUFFIX "${PYTHON_MODULE_EXTENSION}"
      )

    install(TARGETS colonio_py
      RUNTIME DESTINATION ${PROJECT_SOURCE_DIR}/local/bin
      LIBRARY DESTINATION ${PROJECT_SOURCE_DIR}/local/lib
      ARCHIVE DESTINATION ${PROJECT_SOURCE_DIR}/local/lib)
  endif()

  # Export file list for another module.
  foreach(f IN LISTS HEAD_ERFILES COLONIO_FILES)
    list(APPEND CORE_FILES "${PROJECT_SOURCE_DIR}/src/${f}")
  endforeach()
  set(CORE_FILES "${CORE_FILES}" CACHE STRING "core files" FORCE)
endif()
