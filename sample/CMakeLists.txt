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
