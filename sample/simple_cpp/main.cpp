#ifndef NDEBUG
#  include <glog/logging.h>
#endif

#include <uv.h>

#include <cmath>
#include <cstdlib>
#include <iostream>

#include "colonio/colonio.hpp"

static const double C_EARTH = 2.0 * M_PI / (40075.0 * 1000);
static const double R_1M    = M_PI / C_EARTH;

class MyColonio : public colonio::Colonio {
 public:
  MyColonio() : Colonio() {
  }

  void on_output_log(colonio::LogLevel level, const std::string& message) override {
    time_t now = time(nullptr);
    if (level == colonio::LogLevel::INFO) {
      std::cout << ctime(&now) << " - " << message << std::endl;
    } else {
      std::cerr << ctime(&now) << " - " << message << std::endl;
    }
  }
};

MyColonio my_colonio;
colonio::Pubsub2D* db;

uv_timer_t timer_handler;

void on_timer(uv_timer_t* handle) {
  db->publish("me", M_PI * 139.7604131 / 180.0, M_PI * 35.6858593 / 180.0, R_1M * 100, colonio::Value("hello"));
}

int main(int argc, char* argv[]) {
#ifndef NDEBUG
  google::InitGoogleLogging(argv[0]);
  google::InstallFailureSignalHandler();
#endif
  uv_loop_t* loop = uv_default_loop();

  try {
    my_colonio.connect("http://coloniodev:8080/colonio/core.json", "");

    std::cout << "Connect success." << std::endl;
    std::cout << "Local nid is " << my_colonio.get_local_nid() << std::endl;

    my_colonio.set_position(M_PI * 139.7604131 / 180.0, M_PI * 35.6858593 / 180.0);

    db = &my_colonio.access_pubsub_2d("");
    db->on("me", [](const colonio::Value& value) { std::cout << value.get<std::string>() << std::endl; });

    uv_timer_init(loop, &timer_handler);
    uv_timer_start(&timer_handler, on_timer, 1000, 1000);

    uv_run(loop, UV_RUN_DEFAULT);

  } catch (colonio::Exception& e) {
    std::cout << e.what() << std::endl;
  }
  return EXIT_SUCCESS;
}
