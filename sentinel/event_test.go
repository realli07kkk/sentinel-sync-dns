package sentinel

import "testing"

func TestParseSwitchMaster(t *testing.T) {
	event, err := ParseSwitchMaster("master-a 10.0.0.1 6379 10.0.0.2 6379")
	if err != nil {
		t.Fatalf("ParseSwitchMaster returned error: %v", err)
	}

	if event.MasterName != "master-a" {
		t.Fatalf("unexpected master name: %s", event.MasterName)
	}
	if event.NewIP != "10.0.0.2" {
		t.Fatalf("unexpected new ip: %s", event.NewIP)
	}
	if event.Key() != "switch-master:master-a:10.0.0.1:6379:10.0.0.2:6379" {
		t.Fatalf("unexpected event key: %s", event.Key())
	}
}

func TestSwitchMasterEventKeyDistinguishesDifferentTransitionsToSameNewIP(t *testing.T) {
	eventA, err := ParseSwitchMaster("master-a 10.0.0.1 6379 10.0.0.2 6379")
	if err != nil {
		t.Fatalf("ParseSwitchMaster returned error: %v", err)
	}

	eventB, err := ParseSwitchMaster("master-a 10.0.0.3 6379 10.0.0.2 6379")
	if err != nil {
		t.Fatalf("ParseSwitchMaster returned error: %v", err)
	}

	if eventA.Key() == eventB.Key() {
		t.Fatalf("expected different transitions to have different keys, got %s", eventA.Key())
	}
}

func TestParseSwitchMasterRejectsInvalidPayload(t *testing.T) {
	if _, err := ParseSwitchMaster("master-a only-three-fields"); err == nil {
		t.Fatalf("expected invalid payload error")
	}
}

func TestShouldHandleMaster(t *testing.T) {
	filter := BuildMasterFilter([]string{"master-a", " master-b "})

	if !ShouldHandleMaster(filter, "master-a") {
		t.Fatalf("expected master-a to be accepted")
	}
	if ShouldHandleMaster(filter, "master-x") {
		t.Fatalf("expected master-x to be filtered out")
	}
	if !ShouldHandleMaster(nil, "master-any") {
		t.Fatalf("expected nil filter to accept all masters")
	}
}
