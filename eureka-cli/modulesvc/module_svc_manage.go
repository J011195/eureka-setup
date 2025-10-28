package modulesvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/folio-org/eureka-cli/constant"
	"github.com/folio-org/eureka-cli/helpers"
	"github.com/folio-org/eureka-cli/models"
)

type ModuleManager interface {
	GetDeployedModules(client *client.Client, filters filters.Args) ([]container.Summary, error)
	PullModule(client *client.Client, imageName string) error
	DeployModules(client *client.Client, containers *models.Containers, sidecarImage string, sidecarResources *container.Resources) (map[string]int, error)
	DeployModule(client *client.Client, myContainer *models.Container) error
	UndeployModuleByNamePattern(client *client.Client, pattern string) error
}

type SidecarRequest struct {
	Client           *client.Client
	Containers       *models.Containers
	RegistryModule   *models.RegistryModule
	BackendModule    models.BackendModule
	SidecarImage     string
	NetworkConfig    *network.NetworkingConfig
	SidecarResources *container.Resources
}

func (ms *ModuleSvc) GetDeployedModules(client *client.Client, filters filters.Args) ([]container.Summary, error) {
	deployedModules, err := client.ContainerList(context.Background(), container.ListOptions{
		All:     true,
		Filters: filters,
	})
	if err != nil {
		return nil, err
	}

	return deployedModules, nil
}

func (ms *ModuleSvc) PullModule(client *client.Client, imageName string) error {
	authorizationToken, err := ms.RegistrySvc.GetAuthorizationToken()
	if err != nil {
		return err
	}

	reader, err := client.ImagePull(context.Background(), imageName, image.PullOptions{
		RegistryAuth: authorizationToken,
	})
	if err != nil {
		return err
	}
	defer helpers.CloseReader(reader)
	decoder := json.NewDecoder(reader)

	var event *models.Event
	for {
		if err := decoder.Decode(&event); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		}

		if event.Error == "" {
			current := helpers.ConvertMemory(helpers.BytesToMib, int64(event.ProgressDetail.Current))
			total := helpers.ConvertMemory(helpers.BytesToMib, int64(event.ProgressDetail.Total))
			slog.Debug(ms.Action.Name, "text", "Pulling module", "imageName", imageName, "status", event.Status, "progressCurrent", current, "progressTotal", total)
		} else {
			return fmt.Errorf("pulling module image with error %+v", event.Error)
		}
	}

	return nil
}

func (ms *ModuleSvc) DeployModules(client *client.Client, containers *models.Containers, sidecarImage string, sidecarResources *container.Resources) (map[string]int, error) {
	deployedModules := make(map[string]int)
	networkConfig := helpers.NewModuleNetworkConfig()

	var sidecarWG sync.WaitGroup
	sidecarErrCh := make(chan error, 10)
	for registryName, registryModules := range containers.RegistryModules {
		if len(registryModules) > 0 {
			slog.Info(ms.Action.Name, "text", "Deploying modules", "registry", registryName)
		}

		for _, registryModule := range registryModules {
			// TODO Extract
			managementModule := strings.Contains(registryModule.Name, constant.ManagementModulePattern)
			if (containers.ManagementOnly && !managementModule) || (!containers.ManagementOnly && managementModule) {
				continue
			}

			// TODO Extract
			backendModule, ok := containers.BackendModulesMap[registryModule.Name]
			if !ok || !backendModule.DeployModule {
				continue
			}

			moduleVersion := ms.GetModuleImageVersion(backendModule, registryModule)
			moduleImage := ms.GetModuleImage(moduleVersion, registryModule)
			moduleEnv := ms.GetModuleEnv(containers, registryModule, backendModule)
			container := models.NewModuleContainer(registryModule.Name, moduleImage, moduleEnv, backendModule, networkConfig)
			err := ms.DeployModule(client, container)
			if err != nil {
				return nil, err
			}

			deployedModules[registryModule.Name] = backendModule.ModuleExposedServerPort
			if backendModule.DeploySidecar && sidecarImage != "" {
				sidecarWG.Add(1)
				go ms.deploySidecarAsync(&sidecarWG, sidecarErrCh, &SidecarRequest{
					Client:           client,
					Containers:       containers,
					RegistryModule:   registryModule,
					BackendModule:    backendModule,
					SidecarImage:     sidecarImage,
					NetworkConfig:    networkConfig,
					SidecarResources: sidecarResources,
				})
			}
		}
	}

	go func() {
		sidecarWG.Wait()
		close(sidecarErrCh)
	}()
	for err := range sidecarErrCh {
		return nil, err
	}

	return deployedModules, nil
}

func (ms *ModuleSvc) deploySidecarAsync(wg *sync.WaitGroup, errCh chan<- error, req *SidecarRequest) {
	defer wg.Done()

	sidecarEnv := ms.GetSidecarEnv(req.Containers, req.RegistryModule, req.BackendModule, nil, nil)
	sidecarContainer := models.NewSidecarContainer(req.RegistryModule.SidecarName, req.SidecarImage, sidecarEnv, req.BackendModule, req.NetworkConfig, req.SidecarResources)
	err := ms.DeployModule(req.Client, sidecarContainer)
	if err != nil {
		err := fmt.Errorf("failed to deploy %s sidecar with error %w", req.RegistryModule.SidecarName, err)
		slog.Error(ms.Action.Name, "error", err)
		select {
		case errCh <- err:
		default:
		}
	}
}

func (ms *ModuleSvc) DeployModule(client *client.Client, myContainer *models.Container) error {
	containerName := ms.getContainerName(myContainer)
	if myContainer.PullImage {
		err := ms.PullModule(client, myContainer.Image)
		if err != nil {
			return err
		}
	}

	cr, err := client.ContainerCreate(context.Background(), myContainer.Config, myContainer.HostConfig, myContainer.NetworkConfig, myContainer.Platform, containerName)
	if err != nil {
		return err
	}
	if len(cr.Warnings) > 0 {
		slog.Warn(ms.Action.Name, "text", "Caught module creation with warning", "container", containerName, "warnings", cr.Warnings)
	}

	err = client.ContainerStart(context.Background(), cr.ID, container.StartOptions{})
	if err != nil {
		return err
	}
	slog.Info(ms.Action.Name, "text", "Deployed module", "module", containerName)

	return nil
}

func (ms *ModuleSvc) getContainerName(myContainer *models.Container) string {
	if strings.HasPrefix(myContainer.Name, constant.ManagementModulePattern) {
		return fmt.Sprintf("eureka-%s", myContainer.Name)
	}

	return fmt.Sprintf("eureka-%s-%s", ms.Action.ConfigProfile, myContainer.Name)
}

func (ms *ModuleSvc) UndeployModuleByNamePattern(client *client.Client, pattern string) error {
	deployedModules, err := ms.GetDeployedModules(client, filters.NewArgs(filters.KeyValuePair{
		Key:   "name",
		Value: pattern,
	}))
	if err != nil {
		return err
	}

	for _, deployedModule := range deployedModules {
		err = ms.undeployModule(client, deployedModule)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ms *ModuleSvc) undeployModule(client *client.Client, deployedModule container.Summary) error {
	err := client.NetworkDisconnect(context.Background(), constant.NetworkID, deployedModule.ID, false)
	if err != nil {
		slog.Warn(ms.Action.Name, "text", "Module network is disconnected with warnings", "moduleId", deployedModule.ID, "error", err.Error())
	}

	err = client.ContainerStop(context.Background(), deployedModule.ID, container.StopOptions{
		Signal: "9",
	})
	if err != nil {
		return err
	}

	err = client.ContainerRemove(context.Background(), deployedModule.ID, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
	if err != nil {
		slog.Error(ms.Action.Name, "error", err, "module", deployedModule.ID, "operation", "container remove")
	}
	containerName := strings.ReplaceAll(deployedModule.Names[0], "/", "")
	slog.Info(ms.Action.Name, "text", "Undeployed module", "module", containerName)

	return nil
}
