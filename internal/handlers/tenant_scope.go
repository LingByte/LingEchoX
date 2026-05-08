package handlers

func tenantOwns(rowTenantID, reqTenant uint) bool {
	return rowTenantID == reqTenant
}
