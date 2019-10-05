find_package(PkgConfig)

if(${CMAKE_SYSTEM_NAME} MATCHES "Emscripten")
  # for WebAssembly(emscripten)
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s WASM=1")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s ALLOW_MEMORY_GROWTH=1")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s NO_EXIT_RUNTIME=1")
  # set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s NO_FILESYSTEM=1")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s RESERVED_FUNCTION_POINTERS=32")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s EXTRA_EXPORTED_RUNTIME_METHODS=['addFunction','ccall','cwrap']")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s DISABLE_EXCEPTION_CATCHING=0")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s ERROR_ON_UNDEFINED_SYMBOLS=0")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} --js-library ${CMAKE_CURRENT_SOURCE_DIR}/js/api.js")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -Oz --llvm-ltoo 1")

  # Use -s ASSERTIONS=2 to get some useful information about the function pointer being called, and its type.
  # But, Its causes very slow to load wasm.
  # set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -g4 --source-map-base http://localhost:6931/")
  set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -s EXCEPTION_DEBUG=1 -s DISABLE_EXCEPTION_CATCHING=0")
  set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -s ASSERTIONS=1")
  set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -s DEMANGLE_SUPPORT=1")
  set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -s SAFE_HEAP=1 -s STACK_OVERFLOW_CHECK=2")

  # ProtocolBuffers on WebAssembly
  list(APPEND webassembly_libs "protobuf")

else()
  # for native (macOS or Linux)
  if(${CMAKE_SYSTEM_NAME} MATCHES "Darwin")
    # macOS
    file(READ ${LOCAL_ENV_PATH}/exports_libwebrtc.txt webrtc_libs)
    string(REGEX REPLACE "lib([^.]+)\.a[\r\n]*" "\\1;" webrtc_libs "${webrtc_libs}")
    message("webrtc_libs:${webrtc_libs}")

    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -DWEBRTC_MAC=1 -DWEBRTC_POSIX=1")
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -fvisibility=hidden")
    find_library(CORE_FOUNDATION CoreFoundation)
    list(APPEND webrtc_libs ${CORE_FOUNDATION})
    find_library(FOUNDATION Foundation)
    list(APPEND webrtc_libs ${FOUNDATION})
    find_library(CORE_AUDIO CoreAudio)
    list(APPEND webrtc_libs ${CORE_AUDIO})
    find_library(AUDIO_TOOLBOX AudioToolbox)
    list(APPEND webrtc_libs ${AUDIO_TOOLBOX})
    find_library(CORE_GRAPHICS CoreGraphics)
    list(APPEND webrtc_libs ${CORE_GRAPHICS})

  elseif(${CMAKE_SYSTEM_NAME} MATCHES "Linux")
    # Linux
    list(APPEND webrtc_libs "-Wl,--start-group")

    file(READ ${LOCAL_ENV_PATH}/exports_libwebrtc.txt webrtc_read_tmp)
    string(REGEX REPLACE "lib([^.]+)\.a[\r\n]*" "\\1;" webrtc_read_tmp "${webrtc_read_tmp}")
    list(APPEND webrtc_libs "${webrtc_read_tmp}")

    list(APPEND webrtc_libs "-Wl,--end-group")

    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -DWEBRTC_LINUX=1 -DWEBRTC_POSIX=1")
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -fvisibility=hidden -fvisibility-inlines-hidden")
    pkg_search_module(X11 REQUIRED x11)
    list(APPEND webrtc_libs ${X11_LIBRARIES})
  endif()
  include_directories(${PROJECT_SOURCE_DIR}/local/include/third_party/abseil-cpp)

  # cURL
  find_package(CURL REQUIRED)
  include_directories(${CURL_INCLUDE_DIRS})
  link_directories(${CURL_LIBRARY_DIRS})
  list(APPEND extra_libs ${CURL_LIBRARIES})

  # libuv
  pkg_search_module(UV libuv)
  if(UV_FOUND)
    include_directories(${UV_INCLUDE_DIRS})
    string(REPLACE ";" " " UV_CFLAGS_STR "${UV_CFLAGS}")
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} ${UV_CFLAGS_STR}")
    list(APPEND extra_libs ${UV_LIBRARIES})
  else()
    include_directories(${UV_INCLUDE_DIRS})
    list(APPEND extra_libs ${UV_LIBRARIES})
  endif()

  list(APPEND extra_libs "glog")

  # WebSocket++
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -DASIO_STANDALONE")
  include_directories(${PROJECT_SOURCE_DIR}/local/include/third_party/boringssl/src/include)

  # ProtocolBuffers
  list(APPEND extra_libs "protobuf")
endif()

# Compile option for each files.
set_source_files_properties(
  core/webrtc_context_native.cpp
  core/webrtc_link_native.cpp
  PROPERTIES
  COMPILE_FLAGS " -fno-rtti "
  )

# Core files.
file(GLOB HEADER_FILES "*.hpp")
set(COLONIO_FILES
  core/command.cpp
  core/context.cpp
  core/convert.cpp
  core/coord_system.cpp
  core/coord_system_sphere.cpp
  core/coordinate.cpp
  core/export_c.cpp
  core/logger.cpp
  core/map.cpp
  core/map_paxos/map_paxos.cpp
  core/map_paxos/map_paxos_protocol.pb.cc
  core/module.cpp
  core/module_protocol.pb.cc
  core/node_accessor.cpp
  core/node_accessor_protocol.pb.cc
  core/node_id.cpp
  core/protocol.pb.cc
  core/pubsub_2d.cpp
  core/pubsub_2d/pubsub_2d_impl.cpp
  core/pubsub_2d/pubsub_2d_protocol.pb.cc
  core/routing.cpp
  core/routing_protocol.pb.cc
  core/routing_1d.cpp
  core/routing_2d.cpp
  core/seed_accessor.cpp
  core/seed_accessor_protocol.pb.cc
  core/seed_link.cpp
  core/scheduler.cpp
  core/system_1d.cpp
  core/system_2d.cpp
  core/utils.cpp
  core/value.cpp
  core/value_impl.cpp
  core/colonio.cpp
  core/colonio_impl.cpp
  core/webrtc_context.cpp
  core/webrtc_link.cpp
  )

if(${CMAKE_SYSTEM_NAME} MATCHES "Emscripten")
  list(APPEND COLONIO_FILES core/md5/md5.c)
  list(APPEND COLONIO_FILES core/webrtc_context_wasm.cpp)
  list(APPEND COLONIO_FILES core/webrtc_link_wasm.cpp)
  list(APPEND COLONIO_FILES core/seed_link_websocket_wasm.cpp)

else()
  list(APPEND COLONIO_FILES core/webrtc_context_native.cpp)
  list(APPEND COLONIO_FILES core/webrtc_link_native.cpp)
  list(APPEND COLONIO_FILES core/seed_link_websocket_native.cpp)
endif()

include_directories(${PROJECT_SOURCE_DIR}/src)
include_directories(${PROJECT_SOURCE_DIR}/src/c)
include_directories(${PROJECT_SOURCE_DIR}/src/cpp)

if (${CMAKE_SYSTEM_NAME} MATCHES "Emscripten")
  # WebAssembly
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
  # core
  if (WITH_PYTHON)
    # python
    find_package(pybind11 REQUIRED)
    add_library(colonio MODULE
      python/main.cpp
      ${HEADER_FILES}
      ${COLONIO_FILES}
      )
    target_link_libraries(colonio PRIVATE
      pybind11::module
      ${webrtc_libs}
      ${extra_libs}
      )
    set_target_properties(colonio PROPERTIES
      PREFIX "${PYTHON_MODULE_PREFIX}"
      SUFFIX "${PYTHON_MODULE_EXTENSION}"
      )

    install(TARGETS colonio
      RUNTIME DESTINATION ${PROJECT_SOURCE_DIR}/local/bin
      LIBRARY DESTINATION ${PROJECT_SOURCE_DIR}/local/lib
      ARCHIVE DESTINATION ${PROJECT_SOURCE_DIR}/local/lib)

  else()
    add_library(colonio STATIC
      ${HEADER_FILES}
      ${COLONIO_FILES}
      )

    # c/sample
    add_executable(sample_c
      c/sample.c
      )
    target_link_libraries(sample_c
      colonio
      ${webrtc_libs}
      ${extra_libs}
      )

    # cpp/sample
    add_executable(sample_cpp
      cpp/sample.cpp
      )
    target_link_libraries(sample_cpp
      colonio
      ${webrtc_libs}
      ${extra_libs}
      )

    if (WITH_SIMULATE)
      # Redis
      pkg_check_modules(HIREDIS REQUIRED hiredis)
      include_directories(${HIREDIS_INCLUDE_DIRS})

      # simulate
      add_executable(simulate
        simulate/simulate.cpp
        )
      target_link_libraries(simulate
        colonio
        ${webrtc_libs}
        ${extra_libs}
        ${HIREDIS_LIBRARIES}
        )
    endif()

    install(TARGETS colonio
      RUNTIME DESTINATION ${PROJECT_SOURCE_DIR}/local/bin
      LIBRARY DESTINATION ${PROJECT_SOURCE_DIR}/local/lib
      ARCHIVE DESTINATION ${PROJECT_SOURCE_DIR}/local/lib
      )
  endif()
endif()
