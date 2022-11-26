# #
# # Common core files.
# #
file(GLOB HEADER_FILES "*.hpp")
set(COLONIO_FILES
  core/colonio.cpp
  core/colonio_impl.cpp

  core/colonio.pb.cc
  core/command.cpp
  core/command_manager.cpp
  core/convert.cpp
  core/coord_system.cpp
  core/coord_system_plane.cpp
  core/coord_system_sphere.cpp
  core/coordinate.cpp
  core/error.cpp

  # core/export_c.cpp
  core/kvs.cpp
  core/logger.cpp
  core/messaging.cpp
  core/node_accessor.cpp
  core/network.cpp
  core/node_id.cpp
  core/packet.cpp
  core/random.cpp
  core/routing.cpp
  core/routing_1d.cpp
  core/routing_2d.cpp
  core/seed_accessor.cpp
  core/seed_link.cpp
  core/scheduler.cpp
  core/spread.cpp
  core/user_thread_pool.cpp
  core/utils.cpp
  core/value.cpp
  core/value_impl.cpp
  core/webrtc_context.cpp
  core/webrtc_link.cpp
)

if(${CMAKE_SYSTEM_NAME} MATCHES "Emscripten")
  # #
  # # WebAssembly
  # #
  list(APPEND COLONIO_FILES core/md5/md5.c)
  list(APPEND COLONIO_FILES core/scheduler_wasm.cpp)
  list(APPEND COLONIO_FILES core/seed_link_websocket_wasm.cpp)
  list(APPEND COLONIO_FILES core/webrtc_context_wasm.cpp)
  list(APPEND COLONIO_FILES core/webrtc_link_wasm.cpp)

  add_executable(colonio
    js/main.cpp
    ${HEADER_FILES}
    ${COLONIO_FILES}
  )
  target_link_libraries(colonio
    ${webassembly_libs}
  )

# add_custom_target(install_colonio ALL
# COMMAND "cp" "colonio.js" "colonio.wasm" "${PROJECT_SOURCE_DIR}/local/lib/"
# DEPENDS colonio
# )
else()
  # #
  # # Native
  # #
  list(APPEND COLONIO_FILES core/scheduler_native.cpp)
  list(APPEND COLONIO_FILES core/seed_link_websocket_native.cpp)
  list(APPEND COLONIO_FILES core/webrtc_context_native.cpp)
  list(APPEND COLONIO_FILES core/webrtc_link_native.cpp)

  # Core
  add_library(colonio STATIC
    ${HEADER_FILES}
    ${COLONIO_FILES}
  )

  # file(GLOB PUBLIC_HEADER_FILES "colonio/*.h" "colonio/*.hpp")
  # set_target_properties(colonio PROPERTIES PUBLIC_HEADER "${PUBLIC_HEADER_FILES}")
  # install(TARGETS colonio
  # LIBRARY       DESTINATION lib
  # INCLUDES      DESTINATION include
  # PUBLIC_HEADER DESTINATION include/colonio
  # )

  # Python
  if(WITH_PYTHON)
    find_package(pybind11 REQUIRED)
    add_library(colonio_py MODULE
      python/main.cpp
      ${HEADER_FILES}
      ${COLONIO_FILES}
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

    # install(TARGETS colonio_py
    # RUNTIME DESTINATION ${PROJECT_SOURCE_DIR}/local/bin
    # LIBRARY DESTINATION ${PROJECT_SOURCE_DIR}/local/lib
    # ARCHIVE DESTINATION ${PROJECT_SOURCE_DIR}/local/lib)
  endif()

  # Export file list for another module.
  foreach(f IN LISTS HEAD_ERFILES COLONIO_FILES)
    list(APPEND CORE_FILES "${PROJECT_SOURCE_DIR}/src/${f}")
  endforeach()

  set(CORE_FILES "${CORE_FILES}" CACHE STRING "core files" FORCE)
endif()
