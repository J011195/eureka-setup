package consortiumsvc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/folio-org/eureka-cli/constant"
	"github.com/folio-org/eureka-cli/helpers"
)

type ConsortiumCentralOrderingManager interface {
	EnableCentralOrdering(centralTenant string) error
}

func (cs *ConsortiumSvc) EnableCentralOrdering(centralTenant string) error {
	centralOrderingLookupKey := "ALLOW_ORDERING_WITH_AFFILIATED_LOCATIONS"
	enableCentralOrdering, err := cs.getEnableCentralOrderingByKey(centralTenant, centralOrderingLookupKey)
	if err != nil {
		return err
	}
	if enableCentralOrdering {
		slog.Info(cs.Action.Name, "text", "Central ordering for tenant is already enabled", "tenant", centralTenant)
		return nil
	}

	payload, err := json.Marshal(map[string]any{
		"key":   centralOrderingLookupKey,
		"value": "true",
	})
	if err != nil {
		return err
	}

	requestURL := cs.Action.GetRequestURL(constant.KongPort, "/orders-storage/settings")
	headers := helpers.TenantSecureApplicationJSONHeaders(centralTenant, cs.Action.KeycloakAccessToken)
	err = cs.HTTPClient.PostReturnNoContent(requestURL, payload, headers)
	if err != nil {
		return err
	}
	slog.Info(cs.Action.Name, "text", "Enabled central ordering for tenant", "tenant", centralTenant)

	return nil
}

func (cs *ConsortiumSvc) getEnableCentralOrderingByKey(centralTenant string, key string) (bool, error) {
	requestURL := cs.Action.GetRequestURL(constant.KongPort, fmt.Sprintf("/orders-storage/settings?query=key==%s", key))
	headers := helpers.TenantSecureApplicationJSONHeaders(centralTenant, cs.Action.KeycloakAccessToken)
	resp, err := cs.HTTPClient.GetDecodeReturnMapStringAny(requestURL, headers)
	if err != nil {
		return false, err
	}
	if resp["settings"] == nil || len(resp["settings"].([]any)) == 0 {
		return false, nil
	}

	firstSettings := resp["settings"].([]any)[0]
	value := firstSettings.(map[string]any)["value"].(string)
	enableCentralOrdering, err := strconv.ParseBool(value)
	if err != nil {
		return false, err
	}

	return enableCentralOrdering, nil
}
