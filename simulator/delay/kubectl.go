/*
 * Copyright 2017- Yuji Ito <llamerada.jp@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package delay

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	coreV1 "k8s.io/api/core/v1"
)

var kubectlCmd []string

func init() {
	envVer := os.Getenv("KUBECTL")
	if len(envVer) != 0 {
		kubectlCmd = strings.Split(envVer, " ")
	} else {
		kubectlCmd = []string{"kubectl"}
	}
}

func kubectl(args ...string) ([]byte, error) {
	name := kubectlCmd[0]
	args = append(kubectlCmd[1:], args...)
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl %s: %v %s", strings.Join(args, " "), err, string(out))
	}
	return out, nil
}

func kubeGetPods(namespace, label string) ([]coreV1.Pod, error) {
	args := []string{"get", "pods", "-n", namespace, "-l", label, "-o", "json"}
	out, err := kubectl(args...)
	if err != nil {
		return nil, err
	}

	var pods coreV1.PodList
	if err := json.Unmarshal(out, &pods); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pods: %v", err)
	}

	return pods.Items, nil
}

func kubeGetPodsReady(namespace, label string) ([]coreV1.Pod, error) {
	pods, err := kubeGetPods(namespace, label)
	if err != nil {
		return nil, err
	}

	for _, pod := range pods {
		if pod.Status.Phase != coreV1.PodRunning {
			time.Sleep(3 * time.Second)
			return kubeGetPodsReady(namespace, label)
		}
	}

	return pods, nil
}

func kubeExec(namespace, podName, command string) ([]byte, error) {
	fmt.Println(podName, ">", command)
	args := []string{"exec", "-n", namespace, podName, "-c", "node", "--"}
	args = append(args, strings.Split(command, " ")...)
	out, err := kubectl(args...)
	if err != nil {
		return nil, err
	}
	return out, nil
}
