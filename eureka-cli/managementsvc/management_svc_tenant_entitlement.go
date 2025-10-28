package managementsvc

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/folio-org/eureka-cli/constant"
	"github.com/folio-org/eureka-cli/helpers"
)

type ManagementTenantEntitlementManager interface {
	CreateTenantEntitlement(consortiumName string, tenantType constant.TenantType) error
	RemoveTenantEntitlements(consortiumName string, tenantType constant.TenantType, purgeSchemas bool) error
}

func (ms *ManagementSvc) CreateTenantEntitlement(consortiumName string, tenantType constant.TenantType) error {
	tenantParameters, err := ms.TenantSvc.GetEntitlementTenantParameters(consortiumName)
	if err != nil {
		return err
	}

	applicationName := ms.Action.ConfigApplication["name"].(string)
	applicationVersion := ms.Action.ConfigApplication["version"].(string)
	resp, err := ms.GetTenants(consortiumName, tenantType)
	if err != nil {
		return nil
	}

	requestURL := ms.Action.GetRequestURL(constant.KongPort, fmt.Sprintf("/entitlements?purgeOnRollback=true&ignoreErrors=false&tenantParameters=%s", tenantParameters))
	for _, value := range resp {
		mapEntry := value.(map[string]any)
		tenantName := mapEntry["name"].(string)
		if !helpers.HasTenant(tenantName, ms.Action.ConfigTenants) {
			continue
		}

		applications := []string{fmt.Sprintf("%s-%s", applicationName, applicationVersion)}
		payload, err := json.Marshal(map[string]any{
			"tenantId":     mapEntry["id"].(string),
			"applications": applications,
		})
		if err != nil {
			return err
		}

		err = ms.HTTPClient.PostReturnNoContent(requestURL, payload, map[string]string{})
		if err != nil {
			return err
		}
		slog.Info(ms.Action.Name, "text", "Created tenant entitlement for tenant", "tenant", tenantName)
	}

	return nil
}

func (ms *ManagementSvc) RemoveTenantEntitlements(consortiumName string, tenantType constant.TenantType, purgeSchemas bool) error {
	applicationName := ms.Action.ConfigApplication["name"].(string)
	applicationVersion := ms.Action.ConfigApplication["version"].(string)
	resp, err := ms.GetTenants(consortiumName, tenantType)
	if err != nil {
		return err
	}

	requestURL := ms.Action.GetRequestURL(constant.KongPort, fmt.Sprintf("/entitlements?purge=%t&ignoreErrors=false", purgeSchemas))
	for _, value := range resp {
		mapEntry := value.(map[string]any)
		tenantName := mapEntry["name"].(string)
		if !helpers.HasTenant(tenantName, ms.Action.ConfigTenants) {
			continue
		}
		tenantID := mapEntry["id"].(string)

		var applications []string
		applications = append(applications, fmt.Sprintf("%s-%s", applicationName, applicationVersion))
		payload, err := json.Marshal(map[string]any{
			"tenantId":     tenantID,
			"applications": applications,
		})
		if err != nil {
			return err
		}

		err = ms.HTTPClient.DeleteWithBody(requestURL, payload, map[string]string{})
		if err != nil {
			return err
		}
		slog.Info(ms.Action.Name, "text", "Removed tenant entitlement for tenant", "tenant", tenantName)
	}

	return nil
}
