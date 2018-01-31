package collector

import (
	log "github.com/Sirupsen/logrus"
	rancher "github.com/rancher/go-rancher/v3"
)

const orchestrationName = "cattle-V2.0"

type Project struct {
	Total         int          `json:"total"`
	Ns	   	   	  NsInfo 	   `json:"ns"`
	Workload	  WorkloadInfo `json:"workload"`
	Orchestration LabelCount   `json:"orch"`
}

func (p Project) RecordKey() string {
	return "project"
}

func (p Project) Collect(c *CollectorOpts) interface{} {
	opts := NonRemoved()
	opts.Filters["all"] = "true"

	log.Debug("Collecting Projects")
	list, err := c.Client.Project.List(&opts)

	if err != nil {
		log.Errorf("Failed to get Projects err=%s", err)
		return nil
	}

	total := len(list.Data)
	log.Debugf("  Found %d Projects", total)

	p.Orchestration = make(LabelCount)
	p.Orchestration[orchestrationName] = total
	p.Total = total

	var nsUtils  []float64
	var wlUtils  []float64

	for _, project := range list.Data {
		resource := rancher.Resource{}

		// Namespace
		nsCollection := GetNamespaceCollection(c, project.Links["namespaces"])
		if nsCollection == nil {
			continue
		}
		totalNs := len(nsCollection.Data)
		p.Ns.Update(totalNs)
		nsUtils = append(nsUtils, float64(totalNs))
		p.Ns.UpdateFromCatalog(nsCollection)

		// Workload
		wlCollection := GetWorkloadCollection(c, project.Links["workloads"])
		if wlCollection == nil {
			continue
		}
		totalWl := len(nsCollection.Data)
		p.Workload.Update(totalWl)
		wlUtils = append(wlUtils, float64(totalWl))
	}

	p.Ns.UpdateAvg(nsUtils)
	p.Workload.UpdateAvg(wlUtils)

	return p
}

func init() {
	Register(Project{})
}
