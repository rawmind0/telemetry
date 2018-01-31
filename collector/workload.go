package collector

import (
	log "github.com/Sirupsen/logrus"
	rancher "github.com/rancher/go-rancher/v3"
)

type WorkloadInfo struct {
	WorkloadMin	   int `json:"ns_min"`
	WorkloadMax    int `json:"ns_max"`
	WorkloadTotal  int `json:"ns_total"`
	WorkloadAvg    int `json:"ns_avg"`
}

func (w WorkloadInfo) Update(i int){
	w.WorkloadTotal += i
	w.WorkloadMin = MinButNotZero(w.WorkloadMin, i)
	w.WorkloadMax = Max(w.WorkloadMax, i)
}

func (w WorkloadInfo) UpdateAvg(i []float64) {
	w.WorkloadAvg = Clamp(0, Round(Average(i)), 100)
}

type Workload struct {
	Containers	map[string]interface{}	`json:"containers,omitempty"`
	Id 			string					`json:"id,omitempty"`
	Links		map[string]interface{}	`json:"links,omitempty"`
	Name		string					`json:"name,omitempty"`
	NamespaceId string 					`json:"namespaceId,omitempty"`
	NodeId 		string 					`json:"nodeId,omitempty"`
	ProjectId	string					`json:"projectId,omitempty"`
	Scale 		int 					`json:"scale,omitempty"`
	State		string					`json:"state,omitempty"`
	Uuid		string					`json:"state,omitempty"`
}

type WorkloadCollection struct {
	rancher.Collection
	Data   	[]Workload 		`json:"data,omitempty"`
}

func GetWorkloadCollection(c *CollectorOpts, url string) *WorkloadCollection {
	if url == "" {
		log.Debugf("Workload collection link is empty.")
		return nil
	}

	wlCollection := &WorkloadCollection{}
	version := "workloads"

	resource := rancher.Resource{}
	resource.Links = make(map[string]string)
	resource.Links[version] = url

	err := c.Client.GetLink(resource, version, wlCollection)

	if wlCollection == nil || wlCollection.Type != "collection" {
		log.Debugf("Workload collection not found [%s]", resource.Links[version])
		return nil
	}
	if err != nil {
		log.Debugf("Error getting workload collection [%s] %s", resource.Links[version], err)
		return nil
	}

	return wlCollection
}