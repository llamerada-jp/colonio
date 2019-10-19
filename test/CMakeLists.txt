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

set(core_test_targets
  exception
  logger
  node_id
  utils
  )

foreach(test_target IN LISTS core_test_targets)
  add_executable(${test_target} "core/${test_target}_test.cpp")
  target_link_libraries(${test_target} colonio_test ${GTEST_LDFLAGS} ${webrtc_libs} ${extra_libs})
  target_compile_options(${test_target} PUBLIC ${GTEST_CFLAGS})
  add_test(NAME "${test_target}_test" COMMAND "${test_target}")
endforeach()
