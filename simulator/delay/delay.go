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
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
)

const (
	namespace = "colonio-simulator"
	podLabel  = "app=node"
)

type idPair struct {
	id1, id2 string
}

func ApplyDelay(serverFile, pingsFile string) error {
	pods, err := kubeGetPodsReady(namespace, podLabel)
	if err != nil {
		return fmt.Errorf("failed to get pods: %w", err)
	}

	idLocationMap, err := readServersFile(serverFile)
	if err != nil {
		return fmt.Errorf("failed to read servers file: %v", err)
	}

	podIDs := make([]string, len(pods))
	idSet := make(map[string]any)
	for i, pod := range pods {
		lon := pod.Annotations["longitude"]
		lat := pod.Annotations["latitude"]
		if len(lon) == 0 || len(lat) == 0 {
			panic(fmt.Sprintf("pod %s has no longitude or latitude", pod.Name))
		}
		podLocation, err := newLocationByString(lon, lat)
		if err != nil {
			panic(fmt.Sprintf("failed to parse pod location: %v", err))
		}
		podIDs[i] = closestID(idLocationMap, podLocation)
		idSet[podIDs[i]] = struct{}{}
	}

	delayMap, err := readPingsFile(pingsFile, idSet)
	if err != nil {
		return fmt.Errorf("failed to read pings file: %v", err)
	}

	var lastErr error
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i, pod := range pods {
		delays := make(map[string]float64)
		srcID := podIDs[i]
		for j, dstID := range podIDs {
			if i == j {
				continue
			}

			// find delay value
			delayKey := idPair{srcID, dstID}
			if srcID > dstID {
				delayKey = idPair{dstID, srcID}
			}
			delay, ok := delayMap[delayKey]
			// default delay is 0.1s
			if !ok {
				delay = 0.1
			}

			dstIP := pods[j].Status.PodIP
			delays[dstIP] = delay
		}

		localDelay, ok := delayMap[idPair{srcID, srcID}]
		// default local delay is 0.03s
		if !ok {
			localDelay = 0.03
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			err = setDelayLocal(pod.Name, localDelay, pod.Status.PodIP)
			if err != nil {
				mu.Lock()
				lastErr = fmt.Errorf("failed to set local delay for pod %s: %v", pod.Name, err)
				mu.Unlock()
			}

			err = setDelay(pod.Name, delays)
			if err != nil {
				mu.Lock()
				lastErr = fmt.Errorf("failed to set delay for pod %s: %v", pod.Name, err)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	return lastErr
}

func readServersFile(serverFile string) (map[string]*location, error) {
	// read servers.csv
	serverReader, err := os.Open(serverFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open server file: %v", err)
	}
	defer serverReader.Close()

	rows, err := csv.NewReader(serverReader).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read server file: %v", err)
	}

	idLocationMap := make(map[string]*location)
	for i, row := range rows {
		// skip header
		if i == 0 {
			continue
		}
		if len(row) != 10 {
			panic(fmt.Sprintf("invalid row: %v", row))
		}
		latitude, err := strconv.ParseFloat(row[8], 64)
		if err != nil {
			panic(fmt.Sprintf("failed to parse latitude: %v", err))
		}
		longitude, err := strconv.ParseFloat(row[9], 64)
		if err != nil {
			panic(fmt.Sprintf("failed to parse longitude: %v", err))
		}
		idLocationMap[row[0]] = &location{
			longitude: longitude,
			latitude:  latitude,
		}
	}

	return idLocationMap, nil
}

func readPingsFile(pingsFile string, idSet map[string]any) (map[idPair]float64, error) {
	// read pings.csv
	pingReader, err := os.Open(pingsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open pings file: %v", err)
	}
	defer pingReader.Close()

	idsDelayMap := make(map[idPair]float64)

	reader := csv.NewReader(pingReader)
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		// skip header
		if row[0] == "source" {
			continue
		}

		if len(row) != 7 {
			panic(fmt.Sprintf("invalid row: %v", row))
		}
		id1 := row[0]
		id2 := row[1]
		if id2 < id1 {
			id1, id2 = id2, id1
		}
		delay, err := strconv.ParseFloat(row[4], 64)
		if err != nil {
			panic(fmt.Sprintf("failed to parse delay: %v", err))
		}

		_, ok1 := idSet[id1]
		_, ok2 := idSet[id2]
		if !ok1 || !ok2 {
			continue
		}

		// change unit from ms to s
		idsDelayMap[idPair{id1, id2}] = delay / 1000.0
	}

	return idsDelayMap, nil
}

func closestID(idLocationMap map[string]*location, podLocation *location) string {
	var closestID string
	var closestDistance float64 = 1e10

	for id, location := range idLocationMap {
		distance := podLocation.distance(location)
		if distance < closestDistance {
			closestDistance = distance
			closestID = id
		}
	}

	return closestID
}

func setDelay(podName string, delays map[string]float64) error {
	// delete old delay
	_, err := kubeExec(namespace, podName, "tc qdisc del dev eth0 root")
	if err != nil && !strings.Contains(err.Error(), "Cannot delete qdisc with handle of zero") {
		return fmt.Errorf("failed to set delay: %v", err)
	}

	// set delay
	major := 1
	minor := 1

	_, err = kubeExec(namespace, podName, fmt.Sprintf("tc qdisc add dev eth0 root handle %d: prio bands 9", major*10))
	if err != nil {
		return fmt.Errorf("failed to set root qdisc: %v", err)
	}

	for dstIP, delay := range delays {
		_, err = kubeExec(namespace, podName, fmt.Sprintf("tc qdisc add dev eth0 parent %d:%d handle %d: netem delay %dms", major*10, minor, major*10+minor, int(delay*1000)))
		if err != nil {
			return fmt.Errorf("failed to set delay: %v", err)
		}
		_, err = kubeExec(namespace, podName, fmt.Sprintf("tc filter add dev eth0 parent %d: protocol ip prio %d u32 match ip dst %s flowid %d:%d", major*10, minor, dstIP, major*10, minor))
		if err != nil {
			return fmt.Errorf("failed to set delay: %v", err)
		}

		minor++
		if minor == 9 {
			_, err = kubeExec(namespace, podName, fmt.Sprintf("tc qdisc add dev eth0 parent %d:%d handle %d: prio bands 9", major*10, minor, (major+1)*10))
			if err != nil {
				return fmt.Errorf("failed to set delay: %v", err)
			}
			major++
			minor = 1
		}
	}

	// default is no delay
	_, err = kubeExec(namespace, podName, fmt.Sprintf("tc qdisc add dev eth0 parent %d:%d handle %d: pfifo", major*10, minor, major*10+minor))
	if err != nil {
		return fmt.Errorf("failed to set delay: %v", err)
	}

	return nil
}

func setDelayLocal(podName string, delay float64, ip string) error {
	_, err := kubeExec(namespace, podName, "tc qdisc del dev lo root")
	if err != nil && !strings.Contains(err.Error(), "Cannot delete qdisc with handle of zero") {
		return fmt.Errorf("failed to set delay: %v", err)
	}

	// set local delay
	_, err = kubeExec(namespace, podName, "tc qdisc add dev lo root handle 1: prio bands 4")
	if err != nil {
		return fmt.Errorf("failed to set delay: %v", err)
	}
	for i, dstIP := range []string{"0.0.0.0", "127.0.0.1", ip} {
		num := i + 1
		_, err = kubeExec(namespace, podName, fmt.Sprintf("tc qdisc add dev lo parent 1:%d handle %d0: netem delay %dms", num, num, int(delay*1000)))
		if err != nil {
			return fmt.Errorf("failed to set delay: %v", err)
		}
		_, err = kubeExec(namespace, podName, fmt.Sprintf("tc filter add dev lo parent 1: protocol ip prio %d u32 match ip dst %s flowid 1:%d", num, dstIP, num))
		if err != nil {
			return fmt.Errorf("failed to set delay: %v", err)
		}
	}

	// default is no delay
	_, err = kubeExec(namespace, podName, "tc qdisc add dev lo parent 1:4 handle 40: pfifo")
	if err != nil {
		return fmt.Errorf("failed to set delay: %v", err)
	}

	return nil
}
