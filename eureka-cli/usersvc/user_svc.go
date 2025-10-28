package usersvc

import (
	"fmt"

	"github.com/folio-org/eureka-cli/action"
	"github.com/folio-org/eureka-cli/constant"
	"github.com/folio-org/eureka-cli/helpers"
	"github.com/folio-org/eureka-cli/httpclient"
	"github.com/folio-org/eureka-cli/models"
)

type UserProcessor interface {
	Get(tenantName string, username string) (*models.User, error)
}

type UserSvc struct {
	Action     *action.Action
	HTTPClient httpclient.HTTPClientRunner
}

func New(action *action.Action, httpClient httpclient.HTTPClientRunner) *UserSvc {
	return &UserSvc{Action: action, HTTPClient: httpClient}
}

func (us *UserSvc) Get(tenantName string, username string) (*models.User, error) {
	requestURL := us.Action.GetRequestURL(constant.KongPort, fmt.Sprintf("/users?query=username==%s", username))
	headers := helpers.TenantSecureApplicationJSONHeaders(tenantName, us.Action.KeycloakAccessToken)

	var user models.UserResponse
	err := us.HTTPClient.GetReturnStruct(requestURL, headers, &user)
	if err != nil {
		return nil, err
	}
	if len(user.Users) == 0 {
		return nil, fmt.Errorf("user %s not found in tenant %s", username, tenantName)
	}

	return &user.Users[0], nil
}
