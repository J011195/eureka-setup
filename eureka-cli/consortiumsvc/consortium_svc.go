package consortiumsvc

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/folio-org/eureka-cli/action"
	"github.com/folio-org/eureka-cli/constant"
	"github.com/folio-org/eureka-cli/field"
	"github.com/folio-org/eureka-cli/helpers"
	"github.com/folio-org/eureka-cli/httpclient"
	"github.com/folio-org/eureka-cli/usersvc"
	"github.com/google/uuid"
)

type ConsortiumProcessor interface {
	ConsortiumManager
	ConsortiumTenantHandler
	ConsortiumCentralOrderingManager
}

type ConsortiumManager interface {
	GetConsortiumByName(centralTenant string, consortiumName string) (any, error)
	GetConsortiumCentralTenant(consortiumName string) string
	GetConsortiumUsers(consortiumName string) map[string]any
	GetAdminUsername(centralTenant string, consortiumUsers map[string]any) string
	CreateConsortium(centralTenant string, consortiumName string) (string, error)
}

type ConsortiumSvc struct {
	Action     *action.Action
	HTTPClient httpclient.HTTPClientRunner
	UserSvc    usersvc.UserProcessor
}

func New(action *action.Action, httpClient httpclient.HTTPClientRunner, userSvc usersvc.UserProcessor) *ConsortiumSvc {
	return &ConsortiumSvc{Action: action, HTTPClient: httpClient, UserSvc: userSvc}
}

func (cs *ConsortiumSvc) GetConsortiumByName(centralTenant string, consortiumName string) (any, error) {
	requestURL := cs.Action.GetRequestURL(constant.KongPort, fmt.Sprintf("/consortia?query=name==%s", consortiumName))
	headers := helpers.TenantSecureApplicationJSONHeaders(centralTenant, cs.Action.KeycloakAccessToken)
	resp, err := cs.HTTPClient.GetDecodeReturnMapStringAny(requestURL, headers)
	if err != nil {
		return nil, err
	}
	if resp["consortia"] == nil || len(resp["consortia"].([]any)) == 0 {
		return nil, nil
	}

	return resp["consortia"].([]any)[0], nil
}

func (cs *ConsortiumSvc) GetConsortiumCentralTenant(consortiumName string) string {
	for tenantName, properties := range cs.Action.ConfigTenants {
		if properties == nil || !cs.isValidConsortium(consortiumName, properties) ||
			cs.getSortableIsCentral(properties.(map[string]any)) == 0 {
			continue
		}

		return tenantName
	}

	return ""
}

func (cs *ConsortiumSvc) GetConsortiumUsers(consortiumName string) map[string]any {
	consortiumUsers := make(map[string]any)
	for username, properties := range cs.Action.ConfigUsers {
		if !cs.isValidConsortium(consortiumName, properties) {
			continue
		}
		consortiumUsers[username] = properties
	}

	return consortiumUsers
}

func (cs *ConsortiumSvc) GetAdminUsername(centralTenant string, consortiumUsers map[string]any) string {
	for username, properties := range consortiumUsers {
		tenant := properties.(map[string]any)[field.UsersTenantEntry]
		if tenant != nil && tenant.(string) == centralTenant {
			return username
		}
	}

	return ""
}

func (cs *ConsortiumSvc) getSortableIsCentral(mapEntry map[string]any) int {
	if helpers.GetBool(mapEntry, field.TenantsCentralTenantEntry) {
		return 1
	}

	return 0
}

func (cs *ConsortiumSvc) isValidConsortium(consortiumName string, properties any) bool {
	return properties.(map[string]any)[field.TenantsConsortiumEntry] == consortiumName
}

func (cs *ConsortiumSvc) CreateConsortium(centralTenant string, consortiumName string) (string, error) {
	resp, err := cs.GetConsortiumByName(centralTenant, consortiumName)
	if err != nil {
		return "", err
	}
	if resp != nil {
		consortiumID := resp.(map[string]any)["id"].(string)
		slog.Info(cs.Action.Name, "text", "Consortium is already created", "consortium", consortiumName)
		return consortiumID, nil
	}

	consortiumID := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"id":   consortiumID,
		"name": consortiumName,
	})
	if err != nil {
		return "", err
	}

	requestURL := cs.Action.GetRequestURL(constant.KongPort, "/consortia")
	headers := helpers.TenantSecureApplicationJSONHeaders(centralTenant, cs.Action.KeycloakAccessToken)
	err = cs.HTTPClient.PostReturnNoContent(requestURL, payload, headers)
	if err != nil {
		return "", err
	}
	slog.Info(cs.Action.Name, "text", "Created consortium", "consortium", consortiumName)

	return consortiumID.String(), nil
}
