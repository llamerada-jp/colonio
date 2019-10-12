##
## Common build option for test.
##
# Force debug when test.
set(CMAKE_BUILD_TYPE Debug)
set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -coverage -g -O0")

find_package(PkgConfig)
pkg_search_module(GTEST REQUIRED gtest_main)

include_directories(${PROJECT_SOURCE_DIR}/src)

# Library for test.
add_library(colonio_test STATIC
  ${CORE_FILES}
  )

add_executable(node_id
  core/node_id_test.cpp
  )
target_link_libraries(node_id colonio_test
  ${GTEST_LDFLAGS}
  ${webrtc_libs}
  ${extra_libs}
  )
target_compile_options(node_id PUBLIC ${GTEST_CFLAGS})
add_test(TARGET node_id)
