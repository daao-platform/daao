package auth

// RoleLevel returns the numeric level for a role: viewer=0, admin=1, owner=2.
func RoleLevel(role string) int {
	switch role {
	case "viewer":
		return 0
	case "admin":
		return 1
	case "owner":
		return 2
	default:
		return -1
	}
}

// HasPermission checks if userRole has at least the required permission level.
func HasPermission(userRole, requiredRole string) bool {
	return RoleLevel(userRole) >= RoleLevel(requiredRole)
}
