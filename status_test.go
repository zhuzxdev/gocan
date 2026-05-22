package pcanbasic

import "testing"

func TestStatus_OKBoundary(t *testing.T) {
	if StatusHas(StatusBusOff, StatusOK) {
		t.Error("Has(OK) on non-OK status should be false")
	}
	if !StatusHas(StatusOK, StatusOK) {
		t.Error("Has(OK) on OK status should be true")
	}
}

func TestStatus_SingleBit(t *testing.T) {
	if !StatusHas(StatusBusOff, StatusBusOff) {
		t.Error("expected BUSOFF == BUSOFF")
	}
	if StatusHas(StatusBusOff, StatusBusPassive) {
		t.Error("expected BUSOFF != BUSPASSIVE")
	}
}

func TestStatus_BitMatching(t *testing.T) {
	combined := StatusBusOff | StatusQueueOverrun
	if !StatusHas(combined, StatusBusOff) {
		t.Error("expected BUSOFF bit set")
	}
	if !StatusHas(combined, StatusQueueOverrun) {
		t.Error("expected QOVERRUN bit set")
	}
	if StatusHas(combined, StatusBusPassive) {
		t.Error("expected BUSPASSIVE bit NOT set")
	}
}
