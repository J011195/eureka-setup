package uisvc

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/folio-org/eureka-cli/action"
	"github.com/folio-org/eureka-cli/constant"
	"github.com/folio-org/eureka-cli/field"
	"github.com/go-git/go-git/v5/plumbing"
)

type UIStripesConfigProcessor interface {
	GetStripesBranch() plumbing.ReferenceName
	PrepareStripesConfigJS(tenantName string, configPath string) error
}

func (us *UISvc) GetStripesBranch() plumbing.ReferenceName {
	if action.IsSet(field.ApplicationStripesBranch) {
		branchStr := us.Action.ConfigApplicationStripesBranch
		slog.Info(us.Action.Name, "text", "Found stripes branch in config", "branch", branchStr)
		return plumbing.ReferenceName(branchStr)
	}
	slog.Info(us.Action.Name, "text", "No stripes branch is defined in config, using default branch", "defaultBranch", constant.StripesBranch)

	return constant.StripesBranch
}

func (us *UISvc) PrepareStripesConfigJS(tenantName string, configPath string) error {
	stripesConfigJSFilePath := fmt.Sprintf("%s/stripes.config.js", configPath)
	readFileBytes, err := os.ReadFile(stripesConfigJSFilePath)
	if err != nil {
		return err
	}

	tenantOptions := fmt.Sprintf(`{%[1]s: {name: "%[1]s", clientId: "%[1]s%s"}}`, tenantName, action.GetConfigEnv("KC_LOGIN_CLIENT_SUFFIX", us.Action.ConfigGlobalEnv))
	replaceMap := map[string]string{
		"${kongUrl}":           constant.KongExternalHTTP,
		"${tenantUrl}":         us.Action.Params.PlatformCompleteURL,
		"${keycloakUrl}":       constant.KeycloakExternalHTTP,
		"${hasAllPerms}":       `false`,
		"${isSingleTenant}":    strconv.FormatBool(us.Action.Params.SingleTenant),
		"${tenantOptions}":     tenantOptions,
		"${enableEcsRequests}": strconv.FormatBool(us.Action.Params.EnableECSRequests),
	}

	var newReadFileStr = string(readFileBytes)
	for key, value := range replaceMap {
		if !strings.Contains(newReadFileStr, key) {
			slog.Info(us.Action.Name, "text", "Key not found in stripes.config.js", "key", key)
			continue
		}
		newReadFileStr = strings.ReplaceAll(newReadFileStr, key, value)
	}
	newReadFileStr = strings.ReplaceAll(newReadFileStr, "'@folio/users' : {}", "'@folio/users' : {},\n    '@folio/consortia-settings' : {}")
	fmt.Println()
	fmt.Println("DUMPING stripes.config.js")
	fmt.Println(newReadFileStr)
	fmt.Println()

	err = os.WriteFile(stripesConfigJSFilePath, []byte(newReadFileStr), 0)
	if err != nil {
		return err
	}

	return nil
}
