find_package(GTest REQUIRED)

include_directories(${GTEST_INCLUDE_DIRS})
include_directories(${PROJECT_SOURCE_DIR}/src)
include_directories(${PROJECT_SOURCE_DIR}/src/c)
include_directories(${PROJECT_SOURCE_DIR}/src/cpp)

add_executable(node_id
  core/node_id_test.cpp
  ${PROJECT_SOURCE_DIR}/src/core/node_id.cpp
  )
target_link_libraries(node_id colonio
  ${GTEST_BOTH_LIBRARIES}
  ${webrtc_libs}
  )
gtest_add_tests(TARGET node_id)