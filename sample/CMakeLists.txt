##
## Common build option.
##
list(APPEND extra_libs "glog")

##
## simple_cpp
##
add_executable(simple_cpp
  simple_cpp/main.cpp
  )
target_link_libraries(simple_cpp
  colonio
  ${webrtc_libs}
  ${extra_libs}
  )

##
## simulate
##
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
