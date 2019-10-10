##
## Common build option for test.
##
set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -coverage")

find_package(GTest REQUIRED)

include_directories(${GTEST_INCLUDE_DIRS})
include_directories(${PROJECT_SOURCE_DIR}/src)

# Library for test.
add_library(colonio_test STATIC
  ${CORE_FILES}
  )

add_executable(node_id
  core/node_id_test.cpp
  )
target_link_libraries(node_id colonio_test
  ${GTEST_BOTH_LIBRARIES}
  ${webrtc_libs}
  )
gtest_add_tests(TARGET node_id)