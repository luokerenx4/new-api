package setting

import (
	"sort"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

var (
	openAliceManagedGroupsMutex sync.RWMutex
	openAliceManagedGroups      = []string{}
	openAliceRoutableGroups     = []string{}
)

func OpenAliceManagedGroups2JSONString() string {
	openAliceManagedGroupsMutex.RLock()
	defer openAliceManagedGroupsMutex.RUnlock()
	data, err := common.Marshal(openAliceManagedGroups)
	if err != nil {
		common.SysLog("failed to marshal OpenAlice managed groups: " + err.Error())
		return "[]"
	}
	return string(data)
}

func OpenAliceRoutableGroups2JSONString() string {
	openAliceManagedGroupsMutex.RLock()
	defer openAliceManagedGroupsMutex.RUnlock()
	data, err := common.Marshal(openAliceRoutableGroups)
	if err != nil {
		common.SysLog("failed to marshal OpenAlice routable groups: " + err.Error())
		return "[]"
	}
	return string(data)
}

func GetOpenAliceManagedGroups() []string {
	openAliceManagedGroupsMutex.RLock()
	defer openAliceManagedGroupsMutex.RUnlock()
	return append([]string(nil), openAliceManagedGroups...)
}

func GetOpenAliceRoutableGroups() []string {
	openAliceManagedGroupsMutex.RLock()
	defer openAliceManagedGroupsMutex.RUnlock()
	return append([]string(nil), openAliceRoutableGroups...)
}

func UpdateOpenAliceManagedGroupsByJSONString(groupsJSON string) error {
	return updateOpenAliceGroupsByJSONString(groupsJSON, true)
}

func UpdateOpenAliceRoutableGroupsByJSONString(groupsJSON string) error {
	return updateOpenAliceGroupsByJSONString(groupsJSON, false)
}

func updateOpenAliceGroupsByJSONString(groupsJSON string, managed bool) error {
	groups := []string{}
	if err := common.Unmarshal([]byte(groupsJSON), &groups); err != nil {
		return err
	}
	sort.Strings(groups)
	openAliceManagedGroupsMutex.Lock()
	defer openAliceManagedGroupsMutex.Unlock()
	if managed {
		openAliceManagedGroups = groups
	} else {
		openAliceRoutableGroups = groups
	}
	return nil
}
