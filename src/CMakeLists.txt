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
  core/coord_system_sphere.cpp
  core/coordinate.cpp
  core/core.pb.cc
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

  # pubsub_2d/pubsub_2d_impl.cpp
  # pubsub_2d/pubsub_2d_protocol.pb.cc
  )

if(${CMAKE_SYSTEM_NAME} MATCHES "Emscripten")
  ##
  ## WebAssembly
  ##
  list(APPEND COLONIO_FILES core/md5/md5.c)
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

  if (${CMAKE_BUILD_TYPE} MATCHES "Debug")
    add_custom_target(libcolonio ALL
      COMMAND "cc" "-E" "-C" "-P" "-x" "c" "-Wno-invalid-pp-token" "-o" "libcolonio.js" "-I" "${CMAKE_CURRENT_BINARY_DIR}" "${CMAKE_CURRENT_SOURCE_DIR}/js/outline.js"
      DEPENDS colonio
      SOURCES js/outline.js
      )
  else()
    add_custom_target(libcolonio ALL
      COMMAND "cc" "-D" "NDEBUG" "-E" "-C" "-P" "-x" "c" "-Wno-invalid-pp-token" "-o" "libcolonio.js" "-I" "${CMAKE_CURRENT_BINARY_DIR}" "${CMAKE_CURRENT_SOURCE_DIR}/js/outline.js"
      DEPENDS colonio
      SOURCES js/outline.js
      )
  endif()

else()
  ##
  ## Native
  ##
  list(APPEND COLONIO_FILES core/api_gate_mt.cpp)
  list(APPEND COLONIO_FILES core/webrtc_context_native.cpp)
  list(APPEND COLONIO_FILES core/webrtc_link_native.cpp)
  list(APPEND COLONIO_FILES core/seed_link_websocket_native.cpp)

  # Compile option for each files.
  set_source_files_properties(
    core/webrtc_context_native.cpp
    core/webrtc_link_native.cpp
    PROPERTIES
    COMPILE_FLAGS " -fno-rtti "
    )

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
