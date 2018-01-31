package collector

import (
	log "github.com/Sirupsen/logrus"
	rancher "github.com/rancher/go-rancher/v3"
)

type PodInfo struct {
	PodsMin    int `json:"pods_min"`
	PodsMax    int `json:"pods_max"`
	PodsTotal  int `json:"pods_total"`
	UtilMin    int `json:"util_min"`
	UtilAvg    int `json:"util_avg"`
	UtilMax    int `json:"util_max"`
}

func (p PodInfo) Update(total, util int){
	p.PodsMin = MinButNotZero(p.PodsMin, total)
	p.PodsMax = Max(p.PodsMax, total)
	p.PodsTotal += total
	p.UtilMin = MinButNotZero(p.UtilMin, util)
	p.UtilMax = Max(p.UtilMax, util)
}

func (p PodInfo) UpdateAvg(i []float64) {
	p.UtilAvg = Clamp(0, Round(Average(i)), 100)
}

type Pod struct {
	Containers	[]map[string]interface{}	`json:"containers,omitempty"`
	Id 			string						`json:"id,omitempty"`
	Links		map[string]interface{}		`json:"links,omitempty"`
	Name		string						`json:"name,omitempty"`
	NamespaceId string 						`json:"namespaceId,omitempty"`
	NodeId 		string 						`json:"nodeId,omitempty"`
	ProjectId	string						`json:"projectId,omitempty"`
	State		string						`json:"state,omitempty"`
	Uuid		string						`json:"uuid,omitempty"`
	WorkloadId	string						`json:"workloadId,omitempty"`
}

type PodCollection struct {
	rancher.Collection
	Data   	[]Pod 		`json:"data,omitempty"`
}


func GetPodCollection(c *CollectorOpts, url string) *PodCollection {
	if url == "" {
		log.Debugf("Pod collection link is empty.")
		return nil
	}

	podCollection := &PodCollection{}
	version := "pods"

	resource := rancher.Resource{}
	resource.Links = make(map[string]string)
	resource.Links[version] = url

	err := c.Client.GetLink(resource, version, podCollection)

	if podCollection == nil || podCollection.Type != "collection" {
		log.Debugf("Pod collection not found [%s]", resource.Links[version])
		return nil
	}
	if err != nil {
		log.Debugf("Error getting pod collection [%s] %s", resource.Links[version], err)
		return nil
	}

	return podCollection
}
