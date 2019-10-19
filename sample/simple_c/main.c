
#include <stdio.h>

#include "colonio/colonio.h"

void on_require_invoke(colonio_t* colonio, unsigned int msec) {
  printf("on_require_invoke_task\n");
}

void connect_on_success(colonio_t* colonio) {
  printf("connect success\n");
}

void connect_on_failure(colonio_t* colonio) {
  printf("connect failure\n");
}

int main(int argc, char* argv[]) {
  colonio_t colonio;

  colonio_init(&colonio, on_require_invoke);
  colonio_connect(&colonio, "http://coloniodev:8080/colonio/core.json", "", connect_on_success, connect_on_failure);

  return 0;
}
