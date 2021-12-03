
##
## Common core files.
##
file(GLOB HEADER_FILES "*.hpp")
set(COLONIO_FILES
  core/colonio.cpp
  core/colonio_impl.cpp
  core/colonio_module.cpp
  core/command.cpp
  core/convert.cpp
  core/coord_system.cpp
  core/coord_system_plane.cpp
  core/coord_system_sphere.cpp
  core/coordinate.cpp
  core/error.cpp
  core/export_c.cpp
  core/logger.cpp
  core/map.cpp
  core/map_base.cpp
  core/map_impl.cpp
  core/module_1d.cpp
  core/module_2d.cpp
  core/module_base.cpp
  core/node_accessor.cpp
  core/node_id.cpp
  core/pubsub_2d.cpp
  core/pubsub_2d_base.cpp
  core/pubsub_2d_impl.cpp
  core/random.cpp
  core/routing.cpp
  core/routing_1d.cpp
  core/routing_2d.cpp
  core/seed_accessor.cpp
  core/seed_link.cpp
  core/scheduler.cpp
  core/utils.cpp
  core/value.cpp
  core/value_impl.cpp
  core/webrtc_context.cpp
  core/webrtc_link.cpp

  map_paxos/map_paxos_module.cpp

  pubsub_2d/pubsub_2d_module.cpp
  )

##
## Protocol Buffer files.
##
file(GLOB_RECURSE PROTO_FILES RELATIVE ${CMAKE_CURRENT_SOURCE_DIR} "*.proto")
message(STATUS "proto files: ${PROTO_FILES}")
foreach(PROTO_FILE IN LISTS PROTO_FILES)
  string(REGEX REPLACE "[.]proto$" ".pb.cc" OUTPUT_SOURCE ${PROTO_FILE})
  string(REGEX REPLACE "[.]proto$" ".pb.h" OUTPUT_HEADER ${PROTO_FILE})
  message(STATUS "custom command: ${PROTO_FILE} -> ${OUTPUT_SOURCE} ${OUTPUT_HEADER}")
  add_custom_command(
    OUTPUT ${OUTPUT_SOURCE} ${OUTPUT_HEADER}
    COMMAND ${LOCAL_ENV_PATH}/bin/protoc --cpp_out=${CMAKE_CURRENT_BINARY_DIR} ${PROTO_FILE}
    COMMAND ${LOCAL_ENV_PATH}/bin/protoc --cpp_out=${CMAKE_CURRENT_SOURCE_DIR} ${PROTO_FILE}
    DEPENDS ${PROTO_FILE}
    WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
    )
  list(APPEND COLONIO_FILES ${OUTPUT_SOURCE})
  list(APPEND HEADER_FILES ${OUTPUT_HEADER})
  if(CMAKE_CXX_COMPILER_ID MATCHES "GNU")
    set_source_files_properties(${OUTPUT_SOURCE} PROPERTIES COMPILE_OPTIONS "-Warray-bounds=0")
  endif()
endforeach()

if(${CMAKE_SYSTEM_NAME} MATCHES "Emscripten")
  ##
  ## WebAssembly
  ##
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
  #   COMMAND "cp" "colonio.js" "colonio.wasm" "${PROJECT_SOURCE_DIR}/local/lib/"
  #   DEPENDS colonio
  #   )

else()
  ##
  ## Native
  ##
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
  #   LIBRARY       DESTINATION lib
  #   INCLUDES      DESTINATION include
  #   PUBLIC_HEADER DESTINATION include/colonio
  #   )
  
  # Python
  if (WITH_PYTHON)
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
    #   RUNTIME DESTINATION ${PROJECT_SOURCE_DIR}/local/bin
    #   LIBRARY DESTINATION ${PROJECT_SOURCE_DIR}/local/lib
    #   ARCHIVE DESTINATION ${PROJECT_SOURCE_DIR}/local/lib)
  endif()

  # Export file list for another module.
  foreach(f IN LISTS HEAD_ERFILES COLONIO_FILES)
    list(APPEND CORE_FILES "${PROJECT_SOURCE_DIR}/src/${f}")
  endforeach()
  set(CORE_FILES "${CORE_FILES}" CACHE STRING "core files" FORCE)
endif()
