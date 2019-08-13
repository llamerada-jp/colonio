cmake_minimum_required(VERSION 3.1)
project(colonio)

# Read options.
# option(WITH_WEBRTC_SPLIT "Split webrtc process. Turn on if target program linking different version libs from libwebrtc." OFF)
# if (WITH_WEBRTC_SPLIT)
#  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -DWITH_WEBRTC_SPLIT=1")
# endif()
if (NOT ${CMAKE_SYSTEM_NAME} MATCHES "Emscripten")
  option(WITH_PYTHON "Generate library for python." OFF)
  option(WITH_SIMULATE "Generate simulate program." OFF)
endif()

#
set(CMAKE_CXX_STANDARD 14)

if(${CMAKE_SYSTEM_NAME} MATCHES "Emscripten")
  # Add local include path.
  # include_directories(/usr/local/include)
  include_directories(${LOCAL_ENV_PATH}/include)
  include_directories(${LOCAL_ENV_PATH}/wa/include)

  # Add local library path.
  link_directories(${LOCAL_ENV_PATH}/wa/lib)

else()
  # Add local include path.
  include_directories(/usr/local/include)
  include_directories(${LOCAL_ENV_PATH}/include)
  include_directories(${LOCAL_ENV_PATH}/include/webrtc)

  # Add local library path.
  link_directories(${LOCAL_ENV_PATH}/lib)
  link_directories(/usr/local/lib)
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

add_subdirectory(${PROJECT_SOURCE_DIR}/src)
