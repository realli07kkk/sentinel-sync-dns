package sentinel

import (
	"fmt"
	"strings"
)

type SwitchMasterEvent struct {
	MasterName string
	OldIP      string
	OldPort    string
	NewIP      string
	NewPort    string
	RawPayload string
}

func ParseSwitchMaster(payload string) (*SwitchMasterEvent, error) {
	parts := strings.Fields(payload)
	if len(parts) != 5 {
		return nil, fmt.Errorf("invalid switch-master payload %q", payload)
	}

	return &SwitchMasterEvent{
		MasterName: parts[0],
		OldIP:      parts[1],
		OldPort:    parts[2],
		NewIP:      parts[3],
		NewPort:    parts[4],
		RawPayload: payload,
	}, nil
}

func (e *SwitchMasterEvent) Key() string {
	return fmt.Sprintf(
		"switch-master:%s:%s:%s:%s:%s",
		e.MasterName,
		e.OldIP,
		e.OldPort,
		e.NewIP,
		e.NewPort,
	)
}

func BuildMasterFilter(masterNames []string) map[string]struct{} {
	if len(masterNames) == 0 {
		return nil
	}

	filter := make(map[string]struct{}, len(masterNames))
	for _, masterName := range masterNames {
		trimmed := strings.TrimSpace(masterName)
		if trimmed == "" {
			continue
		}
		filter[trimmed] = struct{}{}
	}
	if len(filter) == 0 {
		return nil
	}
	return filter
}

func ShouldHandleMaster(filter map[string]struct{}, masterName string) bool {
	if len(filter) == 0 {
		return true
	}
	_, ok := filter[masterName]
	return ok
}
