package push

import "testing"

func TestInvalidTokenReason(t *testing.T) {
	for _, reason := range []string{"BadDeviceToken", "DeviceTokenNotForTopic", "Unregistered"} {
		if !invalidTokenReason(reason) {
			t.Fatalf("%s was not treated as permanent", reason)
		}
	}
	if invalidTokenReason("TooManyRequests") {
		t.Fatal("transient APNs response was treated as a bad token")
	}
}
