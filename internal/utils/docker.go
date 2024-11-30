package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	podman "github.com/containers/common/libnetwork/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/go-errors/errors"
	"github.com/spf13/viper"
)

var Docker = NewDocker()

func NewDocker() *client.Client {
	// log.Fatalln("Failed to create Docker client:", nil)

	return nil
}

const (
	DinDHost            = "host.docker.internal"
	CliProjectLabel     = "com.supabase.cli.project"
	composeProjectLabel = "com.docker.compose.project"
)

func DockerNetworkCreateIfNotExists(ctx context.Context, mode container.NetworkMode, labels map[string]string) error {
	// Non-user defined networks should already exist
	if !isUserDefined(mode) {
		return nil
	}
	_, err := Docker.NetworkCreate(ctx, mode.NetworkName(), network.CreateOptions{Labels: labels})
	// if error is network already exists, no need to propagate to user
	if errdefs.IsConflict(err) || errors.Is(err, podman.ErrNetworkExists) {
		return nil
	}
	if err != nil {
		return errors.Errorf("failed to create docker network: %w", err)
	}
	return err
}

func WaitAll[T any](containers []T, exec func(container T) error) []error {
	var wg sync.WaitGroup
	result := make([]error, len(containers))
	for i, container := range containers {
		wg.Add(1)
		go func(i int, container T) {
			defer wg.Done()
			result[i] = exec(container)
		}(i, container)
	}
	wg.Wait()
	return result
}

// NoBackupVolume TODO: encapsulate this state in a class
var NoBackupVolume = false

func DockerRemoveAll(ctx context.Context, w io.Writer, projectId string) error {
	fmt.Fprintln(w, "Stopping containers...")
	args := CliProjectFilter(projectId)
	containers, err := Docker.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: args,
	})
	if err != nil {
		return errors.Errorf("failed to list containers: %w", err)
	}
	// Gracefully shutdown containers
	var ids []string
	for _, c := range containers {
		if c.State == "running" {
			ids = append(ids, c.ID)
		}
	}
	result := WaitAll(ids, func(id string) error {
		if err := Docker.ContainerStop(ctx, id, container.StopOptions{}); err != nil {
			return errors.Errorf("failed to stop container: %w", err)
		}
		return nil
	})
	if err := errors.Join(result...); err != nil {
		return err
	}
	if report, err := Docker.ContainersPrune(ctx, args); err != nil {
		return errors.Errorf("failed to prune containers: %w", err)
	} else if viper.GetBool("DEBUG") {
		fmt.Fprintln(os.Stderr, "Pruned containers:", report.ContainersDeleted)
	}
	// Remove named volumes
	if NoBackupVolume {
		vargs := args.Clone()
		if versions.GreaterThanOrEqualTo(Docker.ClientVersion(), "1.42") {
			// Since docker engine 25.0.3, all flag is required to include named volumes.
			// https://github.com/docker/cli/blob/master/cli/command/volume/prune.go#L76
			vargs.Add("all", "true")
		}
		if report, err := Docker.VolumesPrune(ctx, vargs); err != nil {
			return errors.Errorf("failed to prune volumes: %w", err)
		} else if viper.GetBool("DEBUG") {
			fmt.Fprintln(os.Stderr, "Pruned volumes:", report.VolumesDeleted)
		}
	}
	// Remove networks.
	if report, err := Docker.NetworksPrune(ctx, args); err != nil {
		return errors.Errorf("failed to prune networks: %w", err)
	} else if viper.GetBool("DEBUG") {
		fmt.Fprintln(os.Stderr, "Pruned network:", report.NetworksDeleted)
	}
	return nil
}

func CliProjectFilter(projectId string) filters.Args {
	if len(projectId) == 0 {
		return filters.NewArgs(
			filters.Arg("label", CliProjectLabel),
		)
	}
	return filters.NewArgs(
		filters.Arg("label", CliProjectLabel+"="+projectId),
	)
}

func GetRegistryAuth() string {
	return ""
}

// Defaults to Supabase public ECR for faster image pull
const defaultRegistry = "public.ecr.aws"

func GetRegistry() string {
	registry := viper.GetString("INTERNAL_IMAGE_REGISTRY")
	if len(registry) == 0 {
		return defaultRegistry
	}
	return strings.ToLower(registry)
}

func GetRegistryImageUrl(imageName string) string {
	registry := GetRegistry()
	if registry == "docker.io" {
		return imageName
	}
	// Configure mirror registry
	parts := strings.Split(imageName, "/")
	imageName = parts[len(parts)-1]
	return registry + "/supabase/" + imageName
}

func DockerImagePull(ctx context.Context, imageTag string, w io.Writer) error {
	return errors.Errorf("failed to pull docker image: %s", imageTag)
}

// Used by unit tests
var timeUnit = time.Second

func DockerImagePullWithRetry(ctx context.Context, image string, retries int) error {
	err := DockerImagePull(ctx, image, os.Stderr)
	for i := 0; i < retries; i++ {
		if err == nil || errors.Is(ctx.Err(), context.Canceled) {
			break
		}
		fmt.Fprintln(os.Stderr, err)
		period := time.Duration(2<<(i+1)) * timeUnit
		fmt.Fprintf(os.Stderr, "Retrying after %v: %s\n", period, image)
		time.Sleep(period)
		err = DockerImagePull(ctx, image, os.Stderr)
	}
	return err
}

func DockerPullImageIfNotCached(ctx context.Context, imageName string) error {
	imageUrl := GetRegistryImageUrl(imageName)
	if _, _, err := Docker.ImageInspectWithRaw(ctx, imageUrl); err == nil {
		return nil
	} else if !client.IsErrNotFound(err) {
		return errors.Errorf("failed to inspect docker image: %w", err)
	}
	return DockerImagePullWithRetry(ctx, imageUrl, 2)
}

var suggestDockerInstall = "Docker Desktop is a prerequisite for local development. Follow the official docs to install: https://docs.docker.com/desktop"

func DockerStart(ctx context.Context, config container.Config, hostConfig container.HostConfig, networkingConfig network.NetworkingConfig, containerName string) (string, error) {
	return "", errors.Errorf("docker is not available in WebContainer")
}

func DockerRemove(containerId string) {
	if err := Docker.ContainerRemove(context.Background(), containerId, container.RemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to remove container:", containerId, err)
	}
}

type DockerJob struct {
	Image string
	Env   []string
	Cmd   []string
}

func DockerRunJob(ctx context.Context, job DockerJob, stdout, stderr io.Writer) error {
	return DockerRunOnceWithStream(ctx, job.Image, job.Env, job.Cmd, stdout, stderr)
}

// Runs a container image exactly once, returning stdout and throwing error on non-zero exit code.
func DockerRunOnce(ctx context.Context, image string, env []string, cmd []string) (string, error) {
	stderr := GetDebugLogger()
	var out bytes.Buffer
	err := DockerRunOnceWithStream(ctx, image, env, cmd, &out, stderr)
	return out.String(), err
}

func DockerRunOnceWithStream(ctx context.Context, image string, env, cmd []string, stdout, stderr io.Writer) error {
	return DockerRunOnceWithConfig(ctx, container.Config{
		Image: image,
		Env:   env,
		Cmd:   cmd,
	}, container.HostConfig{}, network.NetworkingConfig{}, "", stdout, stderr)
}

func DockerRunOnceWithConfig(ctx context.Context, config container.Config, hostConfig container.HostConfig, networkingConfig network.NetworkingConfig, containerName string, stdout, stderr io.Writer) error {
	// Cannot rely on docker's auto remove because
	//   1. We must inspect exit code after container stops
	//   2. Context cancellation may happen after start
	container, err := DockerStart(ctx, config, hostConfig, networkingConfig, containerName)
	if err != nil {
		return err
	}
	defer DockerRemove(container)
	return DockerStreamLogs(ctx, container, stdout, stderr)
}

func DockerStreamLogs(ctx context.Context, containerId string, stdout, stderr io.Writer) error {
	// Stream logs
	logs, err := Docker.ContainerLogs(ctx, containerId, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return errors.Errorf("failed to read docker logs: %w", err)
	}
	defer logs.Close()
	if _, err := stdcopy.StdCopy(stdout, stderr, logs); err != nil {
		return errors.Errorf("failed to copy docker logs: %w", err)
	}
	// Check exit code
	resp, err := Docker.ContainerInspect(ctx, containerId)
	if err != nil {
		return errors.Errorf("failed to inspect docker container: %w", err)
	}
	if resp.State.ExitCode > 0 {
		return errors.Errorf("error running container: exit %d", resp.State.ExitCode)
	}
	return nil
}

func DockerStreamLogsOnce(ctx context.Context, containerId string, stdout, stderr io.Writer) error {
	logs, err := Docker.ContainerLogs(ctx, containerId, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return errors.Errorf("failed to read docker logs: %w", err)
	}
	defer logs.Close()
	if _, err := stdcopy.StdCopy(stdout, stderr, logs); err != nil {
		return errors.Errorf("failed to copy docker logs: %w", err)
	}
	return nil
}

// Exec a command once inside a container, returning stdout and throwing error on non-zero exit code.
func DockerExecOnce(ctx context.Context, containerId string, env []string, cmd []string) (string, error) {
	stderr := io.Discard
	if viper.GetBool("DEBUG") {
		stderr = os.Stderr
	}
	var out bytes.Buffer
	err := DockerExecOnceWithStream(ctx, containerId, "", env, cmd, &out, stderr)
	return out.String(), err
}

func DockerExecOnceWithStream(ctx context.Context, containerId, workdir string, env, cmd []string, stdout, stderr io.Writer) error {
	// Reset shadow database
	exec, err := Docker.ContainerExecCreate(ctx, containerId, container.ExecOptions{
		Env:          env,
		Cmd:          cmd,
		WorkingDir:   workdir,
		AttachStderr: true,
		AttachStdout: true,
	})
	if err != nil {
		return errors.Errorf("failed to exec docker create: %w", err)
	}
	// Read exec output
	resp, err := Docker.ContainerExecAttach(ctx, exec.ID, container.ExecStartOptions{})
	if err != nil {
		return errors.Errorf("failed to exec docker attach: %w", err)
	}
	defer resp.Close()
	// Capture error details
	if _, err := stdcopy.StdCopy(stdout, stderr, resp.Reader); err != nil {
		return errors.Errorf("failed to copy docker logs: %w", err)
	}
	// Get the exit code
	iresp, err := Docker.ContainerExecInspect(ctx, exec.ID)
	if err != nil {
		return errors.Errorf("failed to exec docker inspect: %w", err)
	}
	if iresp.ExitCode > 0 {
		err = errors.New("error executing command")
	}
	return err
}
