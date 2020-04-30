'use strict';

const URL = 'ws://localhost:8080/test';
const TOKEN = '';

function test() {
  let c1 = new colonio.Colonio();
  c1.on('log', (l) => { console.log(l) });

  return c1.connect(URL, TOKEN).then(() => {
    return c1.disconnect();

  }).then(() => {
    let result = document.getElementById('result');
    result.innerText = "SUCCESS";

  }).catch((e) => {
    let result = document.getElementById('result');
    result.innerText = "FAILURE";
    console.log(e);
  });
}

test();
