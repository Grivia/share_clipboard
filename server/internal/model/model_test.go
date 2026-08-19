package model

import "testing"

func TestDeviceRolePermissions(t *testing.T) {
	tests := []struct {
		name      string
		actor     DeviceRole
		target    DeviceRole
		same      bool
		canRevoke bool
		canChange bool
	}{
		{"super admin manages member", DeviceRoleSuperAdmin, DeviceRoleMember, false, true, true},
		{"super admin manages admin", DeviceRoleSuperAdmin, DeviceRoleAdmin, false, true, true},
		{"super admin cannot manage itself", DeviceRoleSuperAdmin, DeviceRoleSuperAdmin, true, false, false},
		{"admin revokes member", DeviceRoleAdmin, DeviceRoleMember, false, true, false},
		{"admin revokes peer admin", DeviceRoleAdmin, DeviceRoleAdmin, false, true, false},
		{"admin cannot revoke super admin", DeviceRoleAdmin, DeviceRoleSuperAdmin, false, false, false},
		{"member cannot manage devices", DeviceRoleMember, DeviceRoleMember, false, false, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CanRevokeDevice(test.actor, test.target, test.same); got != test.canRevoke {
				t.Fatalf("CanRevokeDevice() = %v, want %v", got, test.canRevoke)
			}
			if got := CanChangeDeviceRole(test.actor, test.target, test.same); got != test.canChange {
				t.Fatalf("CanChangeDeviceRole() = %v, want %v", got, test.canChange)
			}
		})
	}
}

func TestAssignableDeviceRoles(t *testing.T) {
	if !ValidAssignableDeviceRole(DeviceRoleAdmin) || !ValidAssignableDeviceRole(DeviceRoleMember) {
		t.Fatal("admin and member roles must be assignable")
	}
	if ValidAssignableDeviceRole(DeviceRoleSuperAdmin) {
		t.Fatal("super admin must not be assignable")
	}
}
