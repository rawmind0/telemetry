package collector

import (
	"strings"
	"strconv"

	log "github.com/Sirupsen/logrus"
	rancher "github.com/rancher/go-rancher/v3"
)

const catalogProto = "catalog://"

type CpuInfo struct {
	CoresMin   int `json:"cores_min"`
	CoresMax   int `json:"cores_max"`
	CoresTotal int `json:"cores_total"`
	UtilMin    int `json:"util_min"`
	UtilAvg    int `json:"util_avg"`
	UtilMax    int `json:"util_max"`
}

func (c CpuInfo) Update(total, util int){
	c.CoresMin = MinButNotZero(c.CoresMin, total)
	c.CoresMax = Max(c.CoresMin, total)
	c.CoresTotal += total		
	c.UtilMin = MinButNotZero(c.UtilMin, util)
	c.UtilMax = Max(c.UtilMax, util)
}

func (c CpuInfo) UpdateAvg(i []float64) {
	c.UtilAvg = Clamp(0, Round(Average(i)), 100)
}

type MemoryInfo struct {
	MinMb   int `json:"mb_min"`
	MaxMb   int `json:"mb_max"`
	TotalMb int `json:"mb_total"`
	UtilMin int `json:"util_min"`
	UtilAvg int `json:"util_avg"`
	UtilMax int `json:"util_max"`
}

func (m MemoryInfo) Update(total, util int){
	m.MinMb = MinButNotZero(m.MinMb, total)
	m.MaxMb = Max(m.MaxMb, total)
	m.TotalMb += total
	m.UtilMin = MinButNotZero(m.UtilMin, util)
	m.UtilMax = Max(m.UtilMax, util)
}

func (m MemoryInfo) UpdateAvg(i []float64) {
	m.UtilAvg = Clamp(0, Round(Average(i)), 100)
}

func GetRawInt(item, sep string) int {
	var result int

	if sep != "" {
		result, _ = strconv.Atoi(strings.Replace(item, sep, "", 1))
	} else {
		result, _ = strconv.Atoi(item)
	}
	
	return result
}

func FromCatalog(s string) bool {
	if strings.Contains(s, catalogProto) {
		return true
	}
	return false
}

func Min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func MinButNotZero(x, y int) int {
	if x == 0 || x > y {
		return y
	}
	return x
}

func Max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func Average(x []float64) float64 {
	var sum float64
	num := len(x)

	if num == 0 {
		return 0.0
	}

	for _, value := range x {
		sum += value
	}

	return sum / float64(num)
}

func Round(f float64) int {
	return int(f + 0.5)
}

func Clamp(min, x, max int) int {
	return Max(min, Min(x, max))
}

func NonRemoved() rancher.ListOpts {
	filters := make(map[string]interface{})
	filters["state_ne"] = "removed"
	filters["limit"] = "-1"

	out := rancher.ListOpts{
		Filters: filters,
	}

	return out
}

func GetSetting(client *rancher.RancherClient, key string) (string, bool) {
	setting, err := client.Setting.ById(key)
	if err != nil {
		log.Errorf("GetSetting(%s): Error: %s", key, err)
		return "", false
	}

	if setting.Value == "" {
		log.Debugf("GetSetting(%s): Not Set", key)
	} else {
		log.Debugf("GetSetting(%s) = %s", key, setting.Value)
	}
	return setting.Value, true
}

func SetSetting(client *rancher.RancherClient, key string, value string) error {
	setting, err := client.Setting.ById(key)
	if err == nil {
		_, err = client.Setting.Update(setting, map[string]interface{}{"value": value})
		if err == nil {
			log.Debugf("UpdateSetting(%s,%s)", key, value)
		} else {
			log.Debugf("UpdateSetting(%s,%s): Error: %s", key, value, err)
		}
		return err
	}

	setting, err = client.Setting.Create(&rancher.Setting{
		Name:  key,
		Value: value,
	})

	if err == nil {
		log.Debugf("CreateSetting(%s,%s)", key, value)
	} else {
		log.Debugf("CreateSetting(%s,%s): Error: %s", key, value, err)
	}
	return err
}

type LabelCount map[string]int

func (m *LabelCount) Increment(k string) {
	if len(k) == 0 {
		k = "(unknown)"
	}

	cur, ok := (*m)[k]
	if ok {
		(*m)[k] = cur + 1
	} else {
		(*m)[k] = 1
	}
}
