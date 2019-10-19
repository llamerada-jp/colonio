#ifndef NDEBUG
#  include <glog/logging.h>
#endif

#include <cmath>
#include <cstdlib>
#include <iostream>

#include "colonio/colonio_libuv.hpp"

static const double C_EARTH = 2.0 * M_PI / (40075.0 * 1000);
static const double R_1M    = M_PI / C_EARTH;

class MyColonio : public colonio_helper::ColonioLibuv {
 public:
  MyColonio() : ColonioLibuv(nullptr) {
  }

  void on_output_log(colonio::LogLevel::Type level, const std::string& message) override {
    time_t now = time(nullptr);
    if (level == colonio::LogLevel::INFO) {
      std::cout << ctime(&now) << " - " << message << std::endl;
    } else {
      std::cerr << ctime(&now) << " - " << message << std::endl;
    }
  }
};

MyColonio mycolonio;
colonio::PubSub2D* db;

uv_timer_t timer_handler;

void on_timer(uv_timer_t* handle) {
  db->publish(
      "me", M_PI * 139.7604131 / 180.0, M_PI * 35.6858593 / 180.0, R_1M * 100, colonio::Value("hello"), []() {},
      [](colonio::PubSub2DFailureReason) {});
}

void connect_on_success(colonio::Colonio& colonio) {
  std::cout << "Connect success." << std::endl;
  std::cout << "My id is " << colonio.get_my_nid() << std::endl;

  colonio.set_position(M_PI * 139.7604131 / 180.0, M_PI * 35.6858593 / 180.0);

  db = &colonio.access_pubsub2d("");
  db->on("me", [](const colonio::Value& value) { std::cout << value.get<std::string>() << std::endl; });

  uv_timer_init(reinterpret_cast<colonio_helper::ColonioLibuv*>(&colonio)->loop, &timer_handler);
  uv_timer_start(&timer_handler, on_timer, 1000, 1000);
}

void connect_on_failure(colonio::Colonio& colonio) {
  std::cout << "Connect failure." << std::endl;
}

int main(int argc, char* argv[]) {
#ifndef NDEBUG
  google::InitGoogleLogging(argv[0]);
  google::InstallFailureSignalHandler();
#endif

  mycolonio.connect("http://coloniodev:8080/colonio/core.json", "", connect_on_success, connect_on_failure);
  mycolonio.run();
  return EXIT_SUCCESS;
}
