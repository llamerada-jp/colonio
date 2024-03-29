# #
# # Common build option for test.
# #
# Force debug when test.
set(CMAKE_BUILD_TYPE Debug)
set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -g -O0")

if("${WITH_COVERAGE}")
  set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -coverage")
endif()

find_package(PkgConfig)
pkg_search_module(GTEST REQUIRED gtest_main)

include(Dart)
include_directories(${PROJECT_SOURCE_DIR}/src)
include_directories(${PROJECT_SOURCE_DIR}/test)

# Library for test.
add_library(colonio_test STATIC
  ${CORE_FILES}
)

# Combination test
set(test_targets_combine
  connect
  export_c
  kvs
  messaging
  spread
)

foreach(test_target IN LISTS test_targets_combine)
  add_executable(combine_${test_target} "combine/${test_target}_test.cpp")
  target_link_libraries(combine_${test_target} colonio_test ${GTEST_LDFLAGS} ${webrtc_libs} ${extra_libs})
  target_compile_options(combine_${test_target} PUBLIC ${GTEST_CFLAGS})
  add_test(NAME "combine_${test_target}_test" COMMAND "combine_${test_target}")
  set_tests_properties("combine_${test_target}_test" PROPERTIES TIMEOUT "${TEST_TIMEOUT}")
endforeach()

# Unit test of core module
set(test_targets_core
  coord_system_plane
  coordinate
  exception
  logger
  node_id
  scheduler_native
  seed_accessor
  seed_link_native
  user_thread_pool
  utils
  value
  value_impl
)

foreach(test_target IN LISTS test_targets_core)
  add_executable(core_${test_target} "core/${test_target}_test.cpp")
  target_link_libraries(core_${test_target} colonio_test ${GTEST_LDFLAGS} ${webrtc_libs} ${extra_libs})
  target_compile_options(core_${test_target} PUBLIC ${GTEST_CFLAGS})
  add_test(NAME "core_${test_target}_test" COMMAND "core_${test_target}")
  set_tests_properties("core_${test_target}_test" PROPERTIES TIMEOUT "${TEST_TIMEOUT}")
endforeach()
