##
## Common build option for test.
##
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

set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -DCOLONIO_SEED_BIN_PATH=\\\"${COLONIO_SEED_BIN_PATH}\\\"")

# Library for test.
add_library(colonio_test STATIC
  ${CORE_FILES}
  )

# Combination test
set(test_targets_combine
  connect
  export_c
  map
  pubsub_2d
  )

foreach(test_target IN LISTS test_targets_combine)
  add_executable(combine_${test_target} "combine/${test_target}_test.cpp")
  target_link_libraries(combine_${test_target} colonio_test ${GTEST_LDFLAGS} ${webrtc_libs} ${extra_libs})
  target_compile_options(combine_${test_target} PUBLIC ${GTEST_CFLAGS})
  add_test(NAME "combine_${test_target}_test" COMMAND "combine_${test_target}")
endforeach()

# Unit test of core module
set(test_targets_core
  coord_system_plane
  exception
  logger
  node_id
  scheduler_native
  utils
  )

foreach(test_target IN LISTS test_targets_core)
  add_executable(core_${test_target} "core/${test_target}_test.cpp")
  target_link_libraries(core_${test_target} colonio_test ${GTEST_LDFLAGS} ${webrtc_libs} ${extra_libs})
  target_compile_options(core_${test_target} PUBLIC ${GTEST_CFLAGS})
  add_test(NAME "core_${test_target}_test" COMMAND "core_${test_target}")
endforeach()
