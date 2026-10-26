package fsm_test

import (
	"testing"

	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

func TestInMemory(t *testing.T) {
	testStorage(t, func(*testing.T) fsm.Storage { return fsm.NewInMemory() })
}
