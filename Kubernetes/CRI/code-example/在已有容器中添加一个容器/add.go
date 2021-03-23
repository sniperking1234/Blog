package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"

	"google.golang.org/grpc"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	unixProtocol = "unix"
)

var runtimeClient runtimeapi.RuntimeServiceClient

func dial(ctx context.Context, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, unixProtocol, addr)
}

func main() {
	id, err := CreateContainer()
	if err == nil {
		startContainer(id)
	}
}

func CreateContainer() (string, error) {
	endpoint := "unix:///run/containerd/containerd.sock"
	addr, dialer, err := GetAddressAndDialer(endpoint)
	if err != nil {
		log.Fatalf("get addr error", err)
	}
	conn, err := grpc.DialContext(context.Background(), addr, grpc.WithInsecure(), grpc.WithContextDialer(dialer))
	if err != nil {
		log.Fatalf("Connect remote runtime %s failed: %v", conn, err)
	}
	runtimeClient = runtimeapi.NewRuntimeServiceClient(conn)
	request := &runtimeapi.CreateContainerRequest{
		PodSandboxId:  "c6b914e993f9b08e28d6e2bc8fa5692e3c6042800d15ee363f32ad35ff476633",
		Config:        &runtimeapi.ContainerConfig{
			Metadata: &runtimeapi.ContainerMetadata{
				Name:    "test-container",
			},
			Image:       &runtimeapi.ImageSpec{Image: "nginx:latest"},
			LogPath: "pod.log",
		},
		SandboxConfig: &runtimeapi.PodSandboxConfig{
			Metadata: &runtimeapi.PodSandboxMetadata{
				Name:      "web-terminal-0",
				Namespace: "default",
				Uid:       "e31171a8-53dc-11eb-b91d-fa163ee9c90f",
			},
			LogDirectory: "/tmp",
		},
	}
	resp, err := runtimeClient.CreateContainer(context.Background(), request)
	if err != nil {
		fmt.Println("err is ", err)
		return "", err
	}
	fmt.Println("resp is ", resp.String())
	return resp.ContainerId, nil
}

func startContainer(containerID string) {
	_, err := runtimeClient.StartContainer(context.Background(), &runtimeapi.StartContainerRequest{
		ContainerId: containerID,
	})
	if err != nil {
		log.Printf("StartContainer %q from runtime service failed: %v", containerID, err)
	}
}

func GetAddressAndDialer(endpoint string) (string, func(ctx context.Context, addr string) (net.Conn, error), error) {
	protocol, addr, err := parseEndpointWithFallbackProtocol(endpoint, unixProtocol)
	if err != nil {
		return "", nil, err
	}
	if protocol != unixProtocol {
		return "", nil, fmt.Errorf("only support unix socket endpoint")
	}

	return addr, dial, nil
}

func parseEndpointWithFallbackProtocol(endpoint string, fallbackProtocol string) (protocol string, addr string, err error) {
	if protocol, addr, err = parseEndpoint(endpoint); err != nil && protocol == "" {
		fallbackEndpoint := fallbackProtocol + "://" + endpoint
		protocol, addr, err = parseEndpoint(fallbackEndpoint)
		if err == nil {
			log.Printf("Using %q as endpoint is deprecated, please consider using full url format %q.", endpoint, fallbackEndpoint)
		}
	}
	return
}

func parseEndpoint(endpoint string) (string, string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", "", err
	}

	switch u.Scheme {
	case "tcp":
		return "tcp", u.Host, nil

	case "unix":
		return "unix", u.Path, nil

	case "":
		return "", "", fmt.Errorf("using %q as endpoint is deprecated, please consider using full url format", endpoint)

	default:
		return u.Scheme, "", fmt.Errorf("protocol %q not supported", u.Scheme)
	}
}
