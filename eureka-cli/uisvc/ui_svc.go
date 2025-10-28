package uisvc

import (
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"

	"github.com/folio-org/eureka-cli/action"
	"github.com/folio-org/eureka-cli/constant"
	"github.com/folio-org/eureka-cli/dockerclient"
	"github.com/folio-org/eureka-cli/gitclient"
	"github.com/folio-org/eureka-cli/helpers"
	"github.com/folio-org/eureka-cli/tenantsvc"
)

type UIProcessor interface {
	UIRepositoryCloner
	UIContainerManager
	UIPackageJSONProcessor
	UIStripesConfigProcessor
}

type UIRepositoryCloner interface {
	CloneAndUpdateRepository(updateCloned bool) (string, error)
}

type UIContainerManager interface {
	PrepareImage(tenantName string) (string, error)
	BuildImage(tenantName string, outputDir string) (string, error)
	DeployContainer(tenantName string, imageName string, externalPort int) error
}

type UISvc struct {
	Action       *action.Action
	GitClient    gitclient.GitClientRunner
	DockerClient dockerclient.DockerClientRunner
	TenantSvc    tenantsvc.TenantProcessor
}

func New(action *action.Action,
	gitClient gitclient.GitClientRunner,
	dockerClient dockerclient.DockerClientRunner,
	tenantSvc tenantsvc.TenantProcessor) *UISvc {
	return &UISvc{Action: action, GitClient: gitClient, DockerClient: dockerClient, TenantSvc: tenantSvc}
}

func (us *UISvc) CloneAndUpdateRepository(updateCloned bool) (string, error) {
	slog.Info(us.Action.Name, "text", "CLONING & UPDATING PLATFORM COMPLETE UI REPOSITORY")
	branch := us.GetStripesBranch()
	repository, err := us.GitClient.PlatformCompleteRepository(branch)
	if err != nil {
		return "", err
	}

	err = us.GitClient.Clone(repository)
	if err != nil {
		return "", err
	}

	if updateCloned {
		err = us.GitClient.ResetHardPullFromOrigin(repository)
		if err != nil {
			return "", err
		}
	}

	return repository.Dir, nil
}

func (us *UISvc) PrepareImage(tenantName string) (string, error) {
	imageName := fmt.Sprintf("platform-complete-ui-%s", tenantName)
	if us.Action.Params.BuildImages {
		outputDir, err := us.CloneAndUpdateRepository(us.Action.Params.UpdateCloned)
		if err != nil {
			return "", err
		}

		return us.BuildImage(tenantName, outputDir)
	}

	finalImageName, err := us.DockerClient.ForcePullImage(imageName)
	if err != nil {
		return "", err
	}

	return finalImageName, nil
}

func (us *UISvc) BuildImage(tenantName string, outputDir string) (string, error) {
	finalImageName := fmt.Sprintf("platform-complete-ui-%s", tenantName)
	slog.Info(us.Action.Name, "text", "Copying UI configs")
	configName := "stripes.config.js"
	err := helpers.CopySingleFile(us.Action.Name, filepath.Join(outputDir, "eureka-tpl", configName), filepath.Join(outputDir, configName))
	if err != nil {
		return "", err
	}

	slog.Info(us.Action.Name, "text", "PreparingUI configs")
	err = us.PrepareStripesConfigJS(tenantName, outputDir)
	if err != nil {
		return "", err
	}

	err = us.PreparePackageJSON(outputDir)
	if err != nil {
		return "", err
	}

	slog.Info(us.Action.Name, "text", "Building UI from a Dockerfile")
	err = helpers.ExecFromDir(exec.Command("docker", "build", "--tag", finalImageName,
		"--build-arg", fmt.Sprintf("OKAPI_URL=%s", constant.KongExternalHTTP),
		"--build-arg", fmt.Sprintf("TENANT_ID=%s", tenantName),
		"--file", "./docker/Dockerfile",
		"--progress", "plain",
		"--no-cache",
		".",
	), outputDir)
	if err != nil {
		return "", err
	}

	return finalImageName, nil
}

func (us *UISvc) DeployContainer(tenantName string, imageName string, externalPort int) error {
	slog.Info(us.Action.Name, "text", "Deploying UI container for tenant", "tenant", tenantName)
	containerName := fmt.Sprintf("eureka-platform-complete-ui-%s", tenantName)
	err := helpers.Exec(exec.Command("docker", "run", "--name", containerName,
		"--hostname", containerName,
		"--publish", fmt.Sprintf("%d:80", externalPort),
		"--restart", "unless-stopped",
		"--cpus", "1",
		"--memory", "35m",
		"--memory-swap", "-1",
		"--detach",
		imageName,
	))
	if err != nil {
		return err
	}

	slog.Info(us.Action.Name, "text", "Connecting UI container for tenant to network", "tenant", tenantName, "network", constant.NetworkID)
	err = helpers.Exec(exec.Command("docker", "network", "connect", constant.NetworkID, containerName))
	if err != nil {
		return err
	}

	return nil
}
