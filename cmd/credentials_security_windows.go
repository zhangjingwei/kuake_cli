//go:build windows

package main

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// secureCredentialPath installs a protected DACL that grants full access only
// to the user represented by the current process token. Windows does not give
// os.Chmod Unix permission semantics, so credential protection must use ACLs.
func secureCredentialPath(path string, directory bool) error {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("resolve current user SID: %w", err)
	}
	userSID := tokenUser.User.Sid.String()
	if userSID == "" {
		return fmt.Errorf("resolve current user SID: empty SID")
	}

	aceFlags := ""
	if directory {
		// Allow newly-created credential files and subdirectories to inherit the
		// same current-user-only access while the directory remains protected.
		aceFlags = "OICI"
	}
	securityDescriptor, err := windows.SecurityDescriptorFromString(
		fmt.Sprintf("D:P(A;%s;FA;;;%s)", aceFlags, userSID),
	)
	if err != nil {
		return fmt.Errorf("build current-user-only DACL: %w", err)
	}
	dacl, _, err := securityDescriptor.DACL()
	if err != nil {
		return fmt.Errorf("read current-user-only DACL: %w", err)
	}
	securityInformation := windows.SECURITY_INFORMATION(
		windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION,
	)
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		securityInformation,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		return fmt.Errorf("set current-user-only DACL: %w", err)
	}
	return nil
}
