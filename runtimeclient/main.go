package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/kubernetes/pkg/kubelet/cri/remote"
)

const (
	containerDPipe = "npipe:////./pipe/containerd-containerd"
)

var (
	pID            = flag.Int("pid", 0, "")
	defaultTimeout = 2 * time.Second
)

func main() {
	flag.Parse()

	res, err := remote.NewRemoteRuntimeService(containerDPipe, defaultTimeout)
	if err != nil {
		log.Fatalf("failed to open connection: %v\n", err)
	}

	containers, err := res.ListContainers(&runtimeapi.ContainerFilter{})
	if err != nil {
		log.Fatalf("failed to get containers: %v\n", err)
	}

	type ContainerInfo struct {
		SandboxID   string `json:"sandboxID"`
		Pid         uint32 `json:"pid"`
		Removing    bool   `json:"removing"`
		SnapshotKey string `json:"snapshotKey"`
		Snapshotter string `json:"snapshotter"`
		RuntimeType string `json:"runtimeType"`
		// RuntimeOptions interface{} `json:"runtimeOptions"`
		// Config         *runtime.ContainerConfig `json:"config"`
		// RuntimeSpec    *runtimespec.Spec        `json:"runtimeSpec"`
	}

	var containerID string
	var podID string
	for _, each := range containers {
		stat, err := res.ContainerStatus(each.Id, true)
		if err != nil {
			log.Printf("failed to get contianer stats: %v\n", err)
		}
		infoStr := stat.Info["info"]

		containerInfo := ContainerInfo{}

		if err := json.Unmarshal([]byte(infoStr), &containerInfo); err != nil {
			log.Printf("failed to unmarshal info: %v\n", err)
			continue
		}

		if uint32(*pID) == containerInfo.Pid {
			containerID = each.Id
			podID = each.PodSandboxId

			break
		}
	}
	fmt.Println(containerID)
	fmt.Println(podID)
}
