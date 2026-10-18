//go:build windows

package main

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsFileAllAccess windows.ACCESS_MASK = 0x001f01ff

func assertStoredCredentialPermissions(t *testing.T, configDir, path string) {
	t.Helper()
	assertCurrentUserOnlyACL(t, configDir, true)
	assertCurrentUserOnlyACL(t, path, false)
}

func assertCurrentUserOnlyACL(t *testing.T, path string, directory bool) {
	t.Helper()
	securityDescriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		t.Fatalf("read ACL for %q: %v", path, err)
	}
	control, _, err := securityDescriptor.Control()
	if err != nil {
		t.Fatalf("read ACL control for %q: %v", path, err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("ACL for %q inherits access from its parent", path)
	}
	dacl, _, err := securityDescriptor.DACL()
	if err != nil {
		t.Fatalf("read DACL for %q: %v", path, err)
	}
	if dacl == nil || dacl.AceCount != 1 {
		if dacl == nil {
			t.Fatalf("DACL for %q is nil", path)
		}
		t.Fatalf("DACL for %q has %d ACEs, want exactly 1", path, dacl.AceCount)
	}

	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatalf("read ACE for %q: %v", path, err)
	}
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
		t.Fatalf("ACE for %q has type %d, want access-allowed", path, ace.Header.AceType)
	}
	if ace.Mask != windowsFileAllAccess {
		t.Fatalf("ACE for %q has access mask %#x, want %#x", path, ace.Mask, windowsFileAllAccess)
	}
	inheritanceFlags := uint8(windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE)
	if directory {
		if ace.Header.AceFlags&inheritanceFlags != inheritanceFlags {
			t.Fatalf("directory ACE for %q does not inherit to children", path)
		}
	} else if ace.Header.AceFlags&inheritanceFlags != 0 {
		t.Fatalf("file ACE for %q unexpectedly has inheritance flags", path)
	}

	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("resolve current user SID: %v", err)
	}
	aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !aceSID.Equals(tokenUser.User.Sid) {
		t.Fatalf("only ACE for %q belongs to %s, want current user %s", path, aceSID.String(), tokenUser.User.Sid.String())
	}
}
