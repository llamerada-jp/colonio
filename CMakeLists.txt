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
  include_directories(${LOCAL_ENV_PATH}/wasm/include)
  include_directories(${PROJECT_SOURCE_DIR}/src)

  # Add local library path.
  link_directories(${LOCAL_ENV_PATH}/wasm/lib)

  # WebAssembly(emscripten) compile options.
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s WASM=1")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s MODULARIZE=1")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s EXPORT_NAME='ColonioModule'")
  # set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s EXPORT_ES6=1")
  # set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s ALLOW_MEMORY_GROWTH=1")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s NO_EXIT_RUNTIME=1")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s NO_FILESYSTEM=1")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s RESERVED_FUNCTION_POINTERS=32")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s EXTRA_EXPORTED_RUNTIME_METHODS=['addFunction','ccall','cwrap']")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s DISABLE_EXCEPTION_CATCHING=0")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s ERROR_ON_UNDEFINED_SYMBOLS=0")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -s USE_PTHREADS=1")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} --pre-js ${CMAKE_CURRENT_SOURCE_DIR}/src/js/outline.js")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} --js-library ${CMAKE_CURRENT_SOURCE_DIR}/src/js/api.js")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -O0 -flto")

  # Use -s ASSERTIONS=2 to get some useful information about the function pointer being called, and its type.
  # But, Its causes very slow to load wasm.
  # set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -g4 --source-map-base http://localhost:6931/")
  set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -s EXCEPTION_DEBUG=1 -s DISABLE_EXCEPTION_CATCHING=0")
  set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -s ASSERTIONS=2")
  set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -s DEMANGLE_SUPPORT=1")
  set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -s SAFE_HEAP=1 -s STACK_OVERFLOW_CHECK=2")
  set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -g")

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
  include_directories(${LOCAL_ENV_PATH}/include/third_party/abseil-cpp)
  include_directories(${PROJECT_SOURCE_DIR}/src)

  # Add local library path.
  link_directories(${LOCAL_ENV_PATH}/lib)
  link_directories(/usr/local/lib)

  list(APPEND webrtc_libs "webrtc")
  if(${CMAKE_SYSTEM_NAME} MATCHES "Darwin")
    # macOS
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -DWEBRTC_MAC=1 -DWEBRTC_POSIX=1")
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -fvisibility=hidden")
    # Extra libs for WebRTC
    find_library(FOUNDATION Foundation)
    list(APPEND webrtc_libs ${FOUNDATION})

  elseif(${CMAKE_SYSTEM_NAME} MATCHES "Linux")
    # Linux
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -DWEBRTC_LINUX=1 -DWEBRTC_POSIX=1")
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -fvisibility=hidden -fvisibility-inlines-hidden")
  endif()

  # OpenSSL
  find_package(OpenSSL REQUIRED)
  include_directories(${OPENSSL_INCLUDE_DIR})
  list(APPEND webrtc_libs ${OPENSSL_LIBRARIES})

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
