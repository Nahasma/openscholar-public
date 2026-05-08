package testutil

import "github.com/openscholar/openscholar/internal/permission"

// AutoApprovePermission creates a permission.Service that auto-approves all requests.
func AutoApprovePermission(sessionID string) permission.Service {
	svc := permission.NewPermissionService()
	svc.AutoApproveSession(sessionID)
	return svc
}
