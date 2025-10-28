package tenantsvc

import (
	"fmt"
	"log/slog"

	"github.com/folio-org/eureka-cli/action"
	"github.com/folio-org/eureka-cli/consortiumsvc"
	"github.com/folio-org/eureka-cli/constant"
	"github.com/folio-org/eureka-cli/field"
	"github.com/folio-org/eureka-cli/helpers"
)

type TenantProcessor interface {
	GetEntitlementTenantParameters(consortiumName string) (string, error)
	SetConfigTenantParams(tenantName string) error
}

type TenantSvc struct {
	Action        *action.Action
	ConsortiumSvc consortiumsvc.ConsortiumProcessor
}

func New(action *action.Action, consortiumSvc consortiumsvc.ConsortiumProcessor) *TenantSvc {
	return &TenantSvc{Action: action, ConsortiumSvc: consortiumSvc}
}

func (ts *TenantSvc) GetEntitlementTenantParameters(consortiumName string) (string, error) {
	if consortiumName == constant.NoneConsortium {
		return "loadReference=true,loadSample=true", nil
	}

	centralTenant := ts.ConsortiumSvc.GetConsortiumCentralTenant(consortiumName)
	if centralTenant == "" {
		return "", fmt.Errorf("%s consortium does not contain a central tenant", consortiumName)
	}

	return fmt.Sprintf("loadReference=true,loadSample=true,centralTenantId=%s", centralTenant), nil
}

func (ts *TenantSvc) SetConfigTenantParams(tenantName string) error {
	if ts.Action.ConfigTenants == nil || ts.Action.ConfigTenants[tenantName] == nil {
		return fmt.Errorf("found not tenant in the config or by %s tenant", tenantName)
	}

	var configTenant = ts.Action.ConfigTenants[tenantName].(map[string]any)
	helpers.SetBool(configTenant, field.TenantsSingleTenantEntry, &ts.Action.Params.SingleTenant)
	helpers.SetBool(configTenant, field.TenantsEnableEcsRequestEntry, &ts.Action.Params.EnableECSRequests)
	helpers.SetString(configTenant, field.TenantsPlatformCompleteURLEntry, &ts.Action.Params.PlatformCompleteURL)
	slog.Info(ts.Action.Name, "text", "Setting default tenant config params", "tenant", tenantName)

	return nil
}
