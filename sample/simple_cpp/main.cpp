#include <cmath>
#include <cstdlib>
#include <iostream>
#include <thread>

#include "colonio/colonio.hpp"

static const double C_EARTH = 2.0 * M_PI / (40075.0 * 1000);
static const double R_1M    = M_PI / C_EARTH;

colonio::Pubsub2D* db;

int main(int argc, char* argv[]) {
  try {
    std::unique_ptr<colonio::Colonio> node(colonio::Colonio::new_instance());
    node->connect("http://coloniodev:8080/colonio/core.json", "");

    std::cout << "Connect success." << std::endl;
    std::cout << "Local nid is " << node->get_local_nid() << std::endl;

    node->set_position(M_PI * 139.7604131 / 180.0, M_PI * 35.6858593 / 180.0);

    db = &(node->access_pubsub_2d(""));
    db->on("me", [](colonio::Pubsub2D&, const colonio::Value& value) {
      std::cout << value.get<std::string>() << std::endl;
    });

    for (int i = 0; i < 10; i++) {
      db->publish("me", M_PI * 139.7604131 / 180.0, M_PI * 35.6858593 / 180.0, R_1M * 100, colonio::Value("hello"));
      std::this_thread::sleep_for(std::chrono::seconds(1));
    }

    node->disconnect();

  } catch (colonio::Error& e) {
    std::cout << e.what() << std::endl;
  }
  return EXIT_SUCCESS;
}
