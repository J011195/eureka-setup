package keycloaksvc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/folio-org/eureka-cli/action"
	"github.com/folio-org/eureka-cli/constant"
	"github.com/folio-org/eureka-cli/helpers"
	"github.com/folio-org/eureka-cli/httpclient"
	"github.com/folio-org/eureka-cli/managementsvc"
	"github.com/folio-org/eureka-cli/vaultclient"
)

type KeycloakProcessor interface {
	KeycloakAdminManager
	KeycloakUserManager
	KeycloakRoleManager
	KeycloakCapabilitySetManager
}

type KeycloakAdminManager interface {
	GetKeycloakAccessToken(tenantName string) (string, error)
	GetKeycloakMasterAccessToken() (string, error)
	UpdateKeycloakPublicClientParams(tenantName string, url string) error
}

type KeycloakSvc struct {
	Action        *action.Action
	HTTPClient    httpclient.HTTPClientRunner
	VaultClient   vaultclient.VaultClientRunner
	ManagementSvc managementsvc.ManagementProcessor
}

func New(action *action.Action,
	httpClient httpclient.HTTPClientRunner,
	vaultClient vaultclient.VaultClientRunner,
	managementSvc managementsvc.ManagementProcessor) *KeycloakSvc {
	return &KeycloakSvc{Action: action, HTTPClient: httpClient, VaultClient: vaultClient, ManagementSvc: managementSvc}
}

func (ks *KeycloakSvc) GetKeycloakAccessToken(tenantName string) (string, error) {
	client, err := ks.VaultClient.Create()
	if err != nil {
		return "", err
	}

	secrets, err := ks.VaultClient.GetSecretKey(client, ks.Action.VaultRootToken, fmt.Sprintf("folio/%s", tenantName))
	if err != nil {
		return "", err
	}

	clientID := action.GetConfigEnv("KC_SERVICE_CLIENT_ID", ks.Action.ConfigGlobalEnv)
	clientSecret := secrets[clientID].(string)
	systemUser := fmt.Sprintf("%s-system-user", tenantName)
	systemUserPassword := secrets[systemUser].(string)

	formData := url.Values{}
	formData.Set("grant_type", "password")
	formData.Set("client_id", clientID)
	formData.Set("client_secret", clientSecret)
	formData.Set("username", systemUser)
	formData.Set("password", systemUserPassword)

	requestURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", constant.KeycloakHTTP, tenantName)
	headers := helpers.ApplicationFormURLEncodedHeaders()
	tokensMap, err := ks.HTTPClient.PostFormDataReturnMapStringAny(requestURL, formData, headers)
	if err != nil {
		return "", err
	}
	if tokensMap["access_token"] == nil {
		return "", fmt.Errorf("access token not found from %s", requestURL)
	}

	return tokensMap["access_token"].(string), nil
}

func (ks *KeycloakSvc) GetKeycloakMasterAccessToken() (string, error) {
	formData := url.Values{}
	formData.Set("grant_type", "password")
	formData.Set("client_id", "admin-cli")
	formData.Set("username", constant.KeycloakAdminUsername)
	formData.Set("password", constant.KeycloakAdminPassword)

	requestURL := fmt.Sprintf("%s/realms/master/protocol/openid-connect/token", constant.KeycloakHTTP)
	headers := helpers.ApplicationFormURLEncodedHeaders()
	resp, err := ks.HTTPClient.PostFormDataReturnMapStringAny(requestURL, formData, headers)
	if err != nil {
		return "", err
	}
	if resp["access_token"] == nil {
		return "", fmt.Errorf("access token not found from %s", requestURL)
	}

	return resp["access_token"].(string), nil
}

func (ks *KeycloakSvc) UpdateKeycloakPublicClientParams(tenantName string, url string) error {
	clientID := fmt.Sprintf("%s%s", tenantName, action.GetConfigEnv("KC_LOGIN_CLIENT_SUFFIX", ks.Action.ConfigGlobalEnv))
	getRequestURL := fmt.Sprintf("%s/admin/realms/%s/clients?clientId=%s", constant.KeycloakHTTP, tenantName, clientID)
	headers := helpers.SecureApplicationJSONHeaders(ks.Action.KeycloakMasterAccessToken)
	resp1, err := ks.HTTPClient.GetRetryDecodeReturnAny(getRequestURL, headers)
	if err != nil {
		return err
	}

	clients := resp1.([]any)
	if len(clients) != 1 {
		return fmt.Errorf("number of found clients by %s client id is not 1", clientID)
	}

	clientUUID := clients[0].(map[string]any)["id"].(string)
	payload, err := json.Marshal(map[string]any{
		"rootUrl":                      url,
		"baseUrl":                      url,
		"adminUrl":                     url,
		"redirectUris":                 []string{fmt.Sprintf("%s/*", url)},
		"webOrigins":                   []string{"/*"},
		"authorizationServicesEnabled": true,
		"serviceAccountsEnabled":       true,
		"attributes": map[string]string{
			"post.logout.redirect.uris": fmt.Sprintf("%s/*", url),
			"login_theme":               "custom-theme",
		},
	})
	if err != nil {
		return err
	}

	putRequestURL := fmt.Sprintf("%s/admin/realms/%s/clients/%s", constant.KeycloakHTTP, tenantName, clientUUID)
	err = ks.HTTPClient.PutReturnNoContent(putRequestURL, payload, headers)
	if err != nil {
		return err
	}
	slog.Info(ks.Action.Name, "text", "Updated keycloak public client in realm", "client", clientID, "realm", tenantName)

	return nil
}
