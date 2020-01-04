cmake_minimum_required(VERSION 3.10)
project(colonio)
find_package(PkgConfig)

# Read options.
option(WITH_PYTHON "Generate library for python. (For native only)" OFF)
option(WITH_SAMPLE "Generate simulate program. (For native only)" OFF)
option(WITH_TEST   "Generate test program. (For native only)" OFF)
option(WITH_COVERAGE "Build test with coverage.(Native only)" OFF)

if(${CMAKE_SYSTEM_NAME} MATCHES "Emscripten")
  ##
  ## Build for WebAssembly environment.
  ##

  # Enable C++14
  # I cannot use CMAKE_CXX_STANDARD when emscripten?
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -std=c++14")

  # Add local include path.
  # include_directories(/usr/local/include)
  include_directories(${LOCAL_ENV_PATH}/include)
  include_directories(${LOCAL_ENV_PATH}/wa/include)
  include_directories(${PROJECT_SOURCE_DIR}/src)

  # Add local library path.
  link_directories(${LOCAL_ENV_PATH}/wa/lib)

  # WebAssembly(emscripten) compile options.
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

  # ProtocolBuffers
  list(APPEND webassembly_libs "protobuf")

  # Build core library.
  add_subdirectory(${PROJECT_SOURCE_DIR}/src)

else()
  ##
  ## Build for native environment.
  ##

  # Enable C++14
  set(CMAKE_CXX_STANDARD 14)

  # Add local include path.
  include_directories(/usr/local/include)
  include_directories(${LOCAL_ENV_PATH}/include)
  include_directories(${PROJECT_SOURCE_DIR}/src)

  # Add local library path.
  link_directories(${LOCAL_ENV_PATH}/lib)
  link_directories(/usr/local/lib)

  if(${CMAKE_SYSTEM_NAME} MATCHES "Darwin")
    # macOS
    file(READ ${LOCAL_ENV_PATH}/exports_libwebrtc.txt webrtc_libs)
    string(REGEX REPLACE "lib([^.]+)\.a[\r\n]*" "\\1;" webrtc_libs "${webrtc_libs}")
    message("webrtc_libs:${webrtc_libs}")

    # Extra libs for WebRTC
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

  # WebSocket++
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -DASIO_STANDALONE")
  include_directories(${PROJECT_SOURCE_DIR}/local/include/third_party/boringssl/src/include)

  # ProtocolBuffers
  list(APPEND extra_libs "protobuf")

  # Build core library.
  add_subdirectory(${PROJECT_SOURCE_DIR}/src)

  # Build samples.
  if(WITH_SAMPLE)
    add_subdirectory(${PROJECT_SOURCE_DIR}/sample)
  endif()

  # Build test programs.
  if(WITH_TEST)
    enable_testing()
    add_subdirectory(${PROJECT_SOURCE_DIR}/test)
  endif()
endif()

# Check for Google Coding Style.
add_custom_target(cpplint
  python
  `which cpplint.py`
  --extensions=cpp,h,hpp
  --linelength=100
  --filter=-legal/copyright,-runtime/references,-build/c++11
  ${PROJECT_SOURCE_DIR}/src/*.cpp ${PROJECT_SOURCE_DIR}/src/*.hpp
  )
