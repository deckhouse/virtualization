package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"os"
	"sort"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "kubevirt.io/api/core/v1"

	cmdproto "vlctl/pkg/api/generated/cmd/proto"
	infoproto "vlctl/pkg/api/generated/info/proto"
)

var (
	supportedCmdVersions = []uint32{1}
)

const (
	shortTimeout time.Duration = 5 * time.Second
)

type LauncherClient interface {
	GetDomain() (map[string]interface{}, bool, error)
	GetDomainStats() (map[string]interface{}, bool, error)
	GetGuestInfo() (*v1.VirtualMachineInstanceGuestAgentInfo, error)
	GetUsers() (v1.VirtualMachineInstanceGuestOSUserList, error)
	GetFilesystems() (v1.VirtualMachineInstanceFileSystemList, error)
	Ping() error
	GuestPing(string, int32) error
	GetQemuVersion() (string, error)
	GetSEVInfo() (*v1.SEVPlatformInfo, error)
	Close()
}

func NewClient(socketPath string) (LauncherClient, error) {
	conn, err := DialSocket(socketPath)
	if err != nil {
		return nil, err
	}

	infoClient := infoproto.NewCmdInfoClient(conn)
	return NewClientWithInfoClient(infoClient, conn)
}

func NewClientWithInfoClient(infoClient infoproto.CmdInfoClient, conn *grpc.ClientConn) (LauncherClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := infoClient.Info(ctx, &infoproto.CmdInfoRequest{})
	if err != nil {
		return nil, fmt.Errorf("could not check cmd server version: %v", err)
	}
	version, err := GetHighestCompatibleVersion(info.SupportedCmdVersions, supportedCmdVersions)
	if err != nil {
		return nil, err
	}

	// create cmd client
	switch version {
	case 1:
		client := cmdproto.NewCmdClient(conn)
		return newV1Client(client, conn), nil
	default:
		return nil, fmt.Errorf("cmd client version %v not implemented yet", version)
	}
}

func GetHighestCompatibleVersion(serverVersions []uint32, clientVersions []uint32) (uint32, error) {
	sort.Slice(serverVersions, func(i, j int) bool { return serverVersions[i] > serverVersions[j] })
	for _, s := range serverVersions {
		for _, c := range clientVersions {
			if s == c {
				return s, nil
			}

		}
	}
	return 0, fmt.Errorf("no compatible version found, server: %v, client: %v", serverVersions, clientVersions)
}

func newV1Client(client cmdproto.CmdClient, conn *grpc.ClientConn) LauncherClient {
	return &VirtLauncherClient{
		v1client: client,
		conn:     conn,
	}
}

type VirtLauncherClient struct {
	v1client cmdproto.CmdClient
	conn     *grpc.ClientConn
}

func (v VirtLauncherClient) GetDomain() (map[string]interface{}, bool, error) {
	var domain map[string]interface{}

	request := &cmdproto.EmptyRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	domainResponse, err := v.v1client.GetDomain(ctx, request)
	var response *cmdproto.Response
	if domainResponse != nil {
		response = domainResponse.Response
	}

	if err = handleError(err, "GetDomain", response); err != nil || domainResponse == nil {
		return domain, false, err
	}

	if domainResponse.Domain != "" {
		if err := json.Unmarshal([]byte(domainResponse.Domain), &domain); err != nil {
			return domain, false, err
		}
	}
	return domain, true, nil
}

func (v VirtLauncherClient) GetDomainStats() (map[string]interface{}, bool, error) {
	var stats map[string]interface{}

	request := &cmdproto.EmptyRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	domainStatsResponse, err := v.v1client.GetDomainStats(ctx, request)
	var response *cmdproto.Response
	if domainStatsResponse != nil {
		response = domainStatsResponse.Response
	}

	if err = handleError(err, "GetDomainStats", response); err != nil || domainStatsResponse == nil {
		return stats, false, err
	}

	if domainStatsResponse.DomainStats != "" {
		if err := json.Unmarshal([]byte(domainStatsResponse.DomainStats), &stats); err != nil {
			return stats, false, err
		}
	}
	return stats, true, nil
}

func (v VirtLauncherClient) GetGuestInfo() (*v1.VirtualMachineInstanceGuestAgentInfo, error) {
	guestInfo := &v1.VirtualMachineInstanceGuestAgentInfo{}

	request := &cmdproto.EmptyRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	gaRespose, err := v.v1client.GetGuestInfo(ctx, request)
	var response *cmdproto.Response
	if gaRespose != nil {
		response = gaRespose.Response
	}

	if err = handleError(err, "GetGuestInfo", response); err != nil || gaRespose == nil {
		return guestInfo, err
	}

	if gaRespose.GuestInfoResponse != "" {
		if err := json.Unmarshal([]byte(gaRespose.GetGuestInfoResponse()), guestInfo); err != nil {
			return guestInfo, err
		}
	}
	return guestInfo, nil
}

func (v VirtLauncherClient) GetUsers() (v1.VirtualMachineInstanceGuestOSUserList, error) {
	var userList []v1.VirtualMachineInstanceGuestOSUser

	request := &cmdproto.EmptyRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	uResponse, err := v.v1client.GetUsers(ctx, request)
	var response *cmdproto.Response
	if uResponse != nil {
		response = uResponse.Response
	}

	if err = handleError(err, "GetUsers", response); err != nil || uResponse == nil {
		return v1.VirtualMachineInstanceGuestOSUserList{}, err
	}

	if uResponse.GetGuestUserListResponse() != "" {
		if err := json.Unmarshal([]byte(uResponse.GetGuestUserListResponse()), &userList); err != nil {
			return v1.VirtualMachineInstanceGuestOSUserList{}, err
		}
	}

	guestUserList := v1.VirtualMachineInstanceGuestOSUserList{
		Items: userList,
	}

	return guestUserList, nil
}

func (v VirtLauncherClient) GetFilesystems() (v1.VirtualMachineInstanceFileSystemList, error) {
	var fsList []v1.VirtualMachineInstanceFileSystem

	request := &cmdproto.EmptyRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	fsResponse, err := v.v1client.GetFilesystems(ctx, request)
	var response *cmdproto.Response
	if fsResponse != nil {
		response = fsResponse.Response
	}

	if err = handleError(err, "GetFilesystems", response); err != nil || fsResponse == nil {
		return v1.VirtualMachineInstanceFileSystemList{}, err
	}

	if fsResponse.GetGuestFilesystemsResponse() != "" {
		if err := json.Unmarshal([]byte(fsResponse.GetGuestFilesystemsResponse()), &fsList); err != nil {
			return v1.VirtualMachineInstanceFileSystemList{}, err
		}
	}

	filesystemList := v1.VirtualMachineInstanceFileSystemList{
		Items: fsList,
	}

	return filesystemList, nil
}

func (v VirtLauncherClient) Ping() error {
	request := &cmdproto.EmptyRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()
	response, err := v.v1client.Ping(ctx, request)

	err = handleError(err, "Ping", response)
	return err
}

func (v VirtLauncherClient) GuestPing(domainName string, timeoutSeconds int32) error {
	request := &cmdproto.GuestPingRequest{
		DomainName:     domainName,
		TimeoutSeconds: timeoutSeconds,
	}
	ctx, cancel := context.WithTimeout(
		context.Background(),
		// we give the context a bit more time as the timeout should kick
		// on the actual execution
		time.Duration(timeoutSeconds)*time.Second+shortTimeout,
	)
	defer cancel()

	_, err := v.v1client.GuestPing(ctx, request)
	return err
}

func (v VirtLauncherClient) GetQemuVersion() (string, error) {
	request := &cmdproto.EmptyRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	versionResponse, err := v.v1client.GetQemuVersion(ctx, request)
	var response *cmdproto.Response
	if versionResponse != nil {
		response = versionResponse.Response
	}
	if err = handleError(err, "GetQemuVersion", response); err != nil {
		return "", err
	}

	if versionResponse != nil && versionResponse.Version != "" {
		return versionResponse.Version, nil
	}

	return "", errors.New("error getting the qemu version")
}

func (v VirtLauncherClient) GetSEVInfo() (*v1.SEVPlatformInfo, error) {
	request := &cmdproto.EmptyRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	sevInfoResponse, err := v.v1client.GetSEVInfo(ctx, request)
	if err = handleError(err, "GetSEVInfo", sevInfoResponse.GetResponse()); err != nil {
		return nil, err
	}

	sevPlatformInfo := &v1.SEVPlatformInfo{}
	if err := json.Unmarshal(sevInfoResponse.GetSevInfo(), sevPlatformInfo); err != nil {
		return nil, err
	}

	return sevPlatformInfo, nil
}

func (v VirtLauncherClient) Close() {
	_ = v.conn.Close()
}

func IsUnimplemented(err error) bool {
	if grpcStatus, ok := status.FromError(err); ok {
		if grpcStatus.Code() == codes.Unimplemented {
			return true
		}
	}
	return false
}
func handleError(err error, cmdName string, response *cmdproto.Response) error {
	if IsDisconnected(err) {
		return err
	} else if IsUnimplemented(err) {
		return err
	} else if err != nil {
		msg := fmt.Sprintf("unknown error encountered sending command %s: %s", cmdName, err.Error())
		return fmt.Errorf(msg)
	} else if response != nil && !response.Success {
		return fmt.Errorf("server error. command %s failed: %q", cmdName, response.Message)
	}
	return nil
}

func IsDisconnected(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, rpc.ErrShutdown) || errors.Is(err, io.ErrUnexpectedEOF) || err == io.EOF {
		return true
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var syscallErr *os.SyscallError
		if errors.As(opErr.Err, &syscallErr) {
			// catches "connection reset by peer"
			if errors.Is(syscallErr.Err, syscall.ECONNRESET) {
				return true
			}
		}
	}

	if grpcStatus, ok := status.FromError(err); ok {

		// see https://github.com/grpc/grpc-go/blob/master/codes/codes.go
		switch grpcStatus.Code() {
		case codes.Canceled:
			// e.g. v1client connection closing
			return true
		}

	}

	return false
}
