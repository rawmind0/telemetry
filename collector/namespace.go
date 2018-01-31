package collector

import (
	log "github.com/Sirupsen/logrus"
	rancher "github.com/rancher/go-rancher/v3"
)

type NsInfo struct {
	NsMin	   	int `json:"ns_min"`
	NsMax      	int `json:"ns_max"`
	NsTotal    	int `json:"ns_total"`
	NsAvg      	int `json:"ns_avg"`
	FromCatalog int `json:"from_catalog"`
}

func (n NsInfo) Update(i int){
	n.NsTotal += i
	n.NsMin = MinButNotZero(n.NsMin, i)
	n.NsMax = Max(n.NsMax, i)
}

func (n NsInfo) UpdateAvg(i []float64) {
	n.NsAvg = Clamp(0, Round(Average(i)), 100)
}

func (n NsInfo) UpdateFromCatalog(nsc *NamespaceCollection) {
	for _, ns := range nsc.Data {
		if FromCatalog(ns.ExternalId) {
			n.FromCatalog++
		}
	}
}

type Namespace struct {
	Annotations	map[string]interface{}	`json:"annotations,omitempty"`
	ExternalId	string					`json:"externalId,omitempty"`
	Id 			string					`json:"id,omitempty"`
	Links		map[string]interface{}	`json:"links,omitempty"`
	Name		string					`json:"name,omitempty"`
	ProjectId	string					`json:"projectId,omitempty"`
	State		string					`json:"state,omitempty"`
	Uuid		string					`json:"state,omitempty"`
}

type NamespaceCollection struct {
	rancher.Collection
	Data   		[]Namespace 		`json:"data,omitempty"`
}

func GetNamespaceCollection(c *CollectorOpts, url string) *NamespaceCollection {
	if url == "" {
		log.Debugf("Namespace collection link is empty.")
		return nil
	}

	nsCollection := &NamespaceCollection{}
	version := "namespaces"

	resource := rancher.Resource{}
	resource.Links = make(map[string]string)
	resource.Links[version] = url

	err := c.Client.GetLink(resource, version, nsCollection)

	if nsCollection == nil || nsCollection.Type != "collection" {
		log.Debugf("Namespace collection not found [%s]", resource.Links[version])
		return nil
	}
	if err != nil {
		log.Debugf("Error getting namespace collection [%s] %s", resource.Links[version], err)
		return nil
	}

	return nsCollection
}