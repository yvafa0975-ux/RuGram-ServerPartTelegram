package ios

import "github.com/iamxvbaba/td/tg"

// NoAppUpdate is the bounded answer used until rugram has an application
// release catalog. It makes iOS keep its installed build and retry on its
// normal schedule instead of retrying a failed RPC.
func NoAppUpdate() tg.HelpAppUpdateClass {
	return &tg.HelpNoAppUpdate{}
}

// DeviceLockedUpdated acknowledges the client-side autolock report. rugram
// currently has no push-notification privacy state to persist from this hint.
func DeviceLockedUpdated() bool { return true }
