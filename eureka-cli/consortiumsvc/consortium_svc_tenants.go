package consortiumsvc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/folio-org/eureka-cli/constant"
	"github.com/folio-org/eureka-cli/helpers"
)

type ConsortiumTenantHandler interface {
	GetSortedConsortiumTenants(consortiumName string) ConsortiumTenants
	CreateConsortiumTenants(centralTenant string, consortiumID string, consortiumTenants ConsortiumTenants, adminUsername string) error
}

type ConsortiumTenant struct {
	Consortium string
	Tenant     string
	IsCentral  int
}

type ConsortiumTenants []*ConsortiumTenant

func (c ConsortiumTenant) String() string {
	if c.IsCentral == 1 {
		return fmt.Sprintf("%s (central)", c.Tenant)
	}

	return c.Tenant
}

func (c ConsortiumTenants) String() string {
	var builder strings.Builder
	for idx, value := range c {
		builder.WriteString(value.Tenant)
		if idx+1 < len(c) {
			builder.WriteString(", ")
		}
	}

	return builder.String()
}

func (cs *ConsortiumSvc) GetSortedConsortiumTenants(consortiumName string) ConsortiumTenants {
	var consortiumTenants ConsortiumTenants
	for tenantName, properties := range cs.Action.ConfigTenants {
		if !cs.isValidConsortium(consortiumName, properties) {
			continue
		}
		if properties == nil {
			consortiumTenants = append(consortiumTenants, &ConsortiumTenant{
				Tenant:    tenantName,
				IsCentral: 0,
			})
			continue
		}

		isCentral := cs.getSortableIsCentral(properties.(map[string]any))
		consortiumTenants = append(consortiumTenants, &ConsortiumTenant{
			Tenant:    tenantName,
			IsCentral: isCentral,
		})
	}

	sort.Slice(consortiumTenants, func(i, j int) bool {
		return consortiumTenants[i].IsCentral > consortiumTenants[j].IsCentral
	})

	return consortiumTenants
}

func (cs *ConsortiumSvc) CreateConsortiumTenants(centralTenant string, consortiumID string, consortiumTenants ConsortiumTenants, adminUsername string) error {
	headers := helpers.TenantSecureApplicationJSONHeaders(centralTenant, cs.Action.KeycloakAccessToken)
	for _, consortiumTenant := range consortiumTenants {
		payload, err := json.Marshal(map[string]any{
			"id":        consortiumTenant.Tenant,
			"code":      consortiumTenant.Tenant[0:3],
			"name":      consortiumTenant.Tenant,
			"isCentral": consortiumTenant.IsCentral,
		})
		if err != nil {
			return err
		}

		resp, err := cs.getConsortiumTenantByIDAndName(centralTenant, consortiumID, consortiumTenant.Tenant)
		if err != nil {
			return err
		}
		if resp != nil {
			slog.Info(cs.Action.Name, "text", "Consortium tenant is already created", "tenant", consortiumTenant.Tenant)
			continue
		}

		var requestURL = fmt.Sprintf("/consortia/%s/tenants", consortiumID)
		if consortiumTenant.IsCentral == 0 {
			user, err := cs.UserSvc.Get(centralTenant, adminUsername)
			if err != nil {
				return err
			}

			requestURL = fmt.Sprintf("/consortia/%s/tenants?adminUserId=%s", consortiumID, user.ID)
		}

		slog.Info(cs.Action.Name, "text", "Trying to create consortium tenant for consortium", "tenant", consortiumTenant.Tenant, "consortium", consortiumID)
		finalRequestURL := cs.Action.GetRequestURL(constant.KongPort, requestURL)
		err = cs.HTTPClient.PostReturnNoContent(finalRequestURL, payload, headers)
		if err != nil {
			return err
		}

		err = cs.checkConsortiumTenantStatus(centralTenant, consortiumID, consortiumTenant.Tenant, headers)
		if err != nil {
			return err
		}
	}

	return nil
}

func (cs *ConsortiumSvc) getConsortiumTenantByIDAndName(centralTenant string, consortiumID string, tenant string) (any, error) {
	requestURL := cs.Action.GetRequestURL(constant.KongPort, fmt.Sprintf("/consortia/%s/tenants", consortiumID))
	headers := helpers.TenantSecureApplicationJSONHeaders(centralTenant, cs.Action.KeycloakAccessToken)
	resp, err := cs.HTTPClient.GetDecodeReturnMapStringAny(requestURL, headers)
	if err != nil {
		return nil, err
	}
	if resp["tenants"] == nil || len(resp["tenants"].([]any)) == 0 {
		return nil, nil
	}

	for _, value := range resp["tenants"].([]any) {
		matchedConsortiumTenant := value.(map[string]any)["name"]
		if matchedConsortiumTenant != nil && matchedConsortiumTenant.(string) == tenant {
			return matchedConsortiumTenant, nil
		}
	}

	return nil, nil
}

func (cs *ConsortiumSvc) checkConsortiumTenantStatus(centralTenant string, consortiumID string, tenantName string, headers map[string]string) error {
	requestURL := fmt.Sprintf("/consortia/%s/tenants/%s", consortiumID, tenantName)
	consortiumTenants, err := cs.HTTPClient.GetDecodeReturnMapStringAny(cs.Action.GetRequestURL(constant.KongPort, requestURL), headers)
	if err != nil {
		return err
	}
	if consortiumTenants == nil {
		return nil
	}

	const (
		IN_PROGRESS           string = "IN_PROGRESS"
		FAILED                string = "FAILED"
		COMPLETED             string = "COMPLETED"
		COMPLETED_WITH_ERRORS string = "COMPLETED_WITH_ERRORS"
	)
	switch consortiumTenants["setupStatus"] {
	case IN_PROGRESS:
		slog.Info(cs.Action.Name, "text", "Waiting for consortium tenant creation", "tenant", tenantName)
		time.Sleep(constant.ConsortiumTenantStatusWait)
		err = cs.checkConsortiumTenantStatus(centralTenant, consortiumID, tenantName, headers)
		if err != nil {
			return err
		}
		return nil
	case FAILED:
	case COMPLETED_WITH_ERRORS:
		return fmt.Errorf("%s consortium tenant not is created", tenantName)
	case COMPLETED:
		isCentral := consortiumTenants["isCentral"]
		slog.Info(cs.Action.Name, "text", "Created consortium tenant for consortium", "tenant", tenantName, "isCentral", isCentral, "consortium", consortiumID)
		return nil
	}

	return nil
}
