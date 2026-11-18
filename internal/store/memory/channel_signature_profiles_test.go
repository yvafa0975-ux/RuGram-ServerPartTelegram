package memory_test

import (
	"rugram/internal/store"
	"rugram/internal/store/memory"
	"rugram/internal/store/storetest"
	"testing"
)

func TestChannelSignatureProfiles(t *testing.T) {
	s := memory.NewChannelStore()
	storetest.ChannelSignatureProfiles(t, s, func() store.ChannelStore { return s }, 101, 102)
}
