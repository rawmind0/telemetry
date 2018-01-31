package collector

import (
	log "github.com/Sirupsen/logrus"
	rancher "github.com/rancher/go-rancher/v3"
	"regexp"
)

type Machine struct {
	Active     int        `json:"active"`
	Total      int        `json:"total"`
	Cpu        CpuInfo    `json:"cpu"`
	Mem        MemoryInfo `json:"mem"`
	Pod        PodInfo 	  `json:"pod"`
	Kernel     LabelCount `json:"kernel"`
	Kubelet    LabelCount `json:"kubelet"`
	Kubeproxy  LabelCount `json:"kubeproxy"`
	Os         LabelCount `json:"os"`
	DockerFull LabelCount `json:"docker_full"`
	Docker     LabelCount `json:"docker"`
	Driver     LabelCount `json:"driver"`
	Role       LabelCount `json:"role"`
}

func (m Machine) RecordKey() string {
	return "machine"
}

func (h Machine) Collect(c *CollectorOpts) interface{} {
	nonRemoved := NonRemoved()

	log.Debug("Collecting Machines")
	machineList, err := c.Client.Machine.List(&nonRemoved)
	if err != nil {
		log.Errorf("Failed to get Machines err=%s", err)
		return nil
	}

	log.Debugf("  Found %d Machines", len(machineList.Data))

	var cpuUtils []float64
	var memUtils []float64
	var podUtils []float64

	h.Kernel = make(LabelCount)
	h.Os = make(LabelCount)
	h.Docker = make(LabelCount)
	h.DockerFull = make(LabelCount)
	h.Driver = make(LabelCount)

	// Machines
	for _, machine := range machineList.Data {
		var utilFloat float64
		var util int


		log.Debugf("  Machine: %s", displayMachineName(machine))

		h.Total++
		if machine.State == "active" {
			h.Active++
		}

		allocatable := machine.Allocatable.(map[string]interface{})
		if allocatable["cpu"] == "0" || allocatable["memory"] == "0" || allocatable["pods"] == "0" {
			log.Debugf("  Skipping Machine with no resources: %s", displayMachineName(machine))
			continue
		}

		requested := machine.Requested.(map[string]interface{})

		// CPU
		totalCores := GetRawInt(allocatable["cpu"], "")
		usedCores := GetRawInt(requested["cpu"],"m")
		utilFloat = float64(usedCores) / float64(totalCores*10)
		util = Round(utilFloat)

		h.Cpu.Update(totalCores, util)
		cpuUtils = append(cpuUtils, utilFloat)
		log.Debugf("    CPU cores=%d, util=%d", totalCores, util)

		// Memory
		totalMemMb := GetRawInt(allocatable["memory"], "Ki")/1024
		usedMemMB := GetRawInt(requested["memory"], "")/1024/1024
		utilFloat = 100 * float64(usedMemMB) / float64(totalMemMb)
		util = Round(utilFloat)

		h.Mem.Update(totalMemMb, util)
		memUtils = append(memUtils, utilFloat)
		log.Debugf("    Mem used=%d, total=%d, util=%d", usedMemMB, totalMemMb, util)

		// Pod
		totalPods := GetRawInt(allocatable["pods"], "")
		usedPods := GetRawInt(requested["pods"], "")
		utilFloat = 100 * float64(usedPods) / float64(totalPods)
		util = Round(utilFloat)

		h.Pod.Update(totalPods, util)
		log.Debugf("    Pod used=%d, total=%d, util=%d", usedPods, totalPods, util)

		// OS
		info, ok := machine.Info.(map[string]interface{})
		if !ok {
			log.Debugf("  Skipping Machine with no Info: %s", displayMachineName(machine))
			continue
		}

		osInfo := info["os"].(map[string]interface{})
		h.Kernel.Increment(osInfo["kernelVersion"].(string))
		h.Os.Increment(osInfo["operatingSystem"].(string))

		dockerFull := osInfo["dockerVersion"].(string)
		docker := regexp.MustCompile("(?i)^docker version (.*)").ReplaceAllString(dockerFull, "v$1")
		docker = regexp.MustCompile("(?i)^(.*), build [0-9a-f]+$").ReplaceAllString(docker, "$1")
		h.DockerFull.Increment(dockerFull)
		h.Docker.Increment(docker)

		kubeInfo := info["kubernetes"].(map[string]interface{})
		h.Kubelet.Increment(osInfo["kubeletVersion"].(string))
		h.Kubeproxy.Increment(osInfo["kubeProxyVersion"].(string))

		// Role
		for _, role := range machine.Role {
			h.Driver.Increment(role)
		}

		// Driver
		machineTemplate, err := c.Client.MachineTemplate.ById(info["machineTemplateId"])
		if err != nil {
			log.Debugf("  Skipping Machine Template with no Driver Info: %s", info["machineTemplateId"])
			continue
		}
		h.Driver.Increment(machineTemplate.Driver)
	}

	h.Cpu.UpdateAvg(cpuUtils)
	h.Mem.UpdateAvg(memUtils)
	h.Pod.UpdateAvg(podUtils)

	return h
}

func init() {
	Register(Machine{})
}

func displayMachineName(m rancher.Machine) string {
	if len(m.Name) > 0 {
		return m.Name
	} else if len(m.Hostname) > 0 {
		return m.Hostname
	} else {
		return "(" + m.Id + ")"
	}
}
