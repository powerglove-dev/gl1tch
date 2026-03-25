package host

import (
	"context"
	"os/exec"

	hclog "github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	"github.com/adam-stokes/orcai/internal/discovery"
	pb "github.com/adam-stokes/orcai/proto/orcai/v1"
)

// handshake is the shared config that host and plugins must agree on.
var handshake = goplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "ORCAI_PLUGIN",
	MagicCookieValue: "orcai",
}

// grpcPlugin bridges go-plugin and our OrcaiPlugin gRPC service.
type grpcPlugin struct {
	goplugin.Plugin
}

func (p *grpcPlugin) GRPCServer(_ *goplugin.GRPCBroker, _ *grpc.Server) error {
	return nil // host never serves
}

func (p *grpcPlugin) GRPCClient(_ context.Context, _ *goplugin.GRPCBroker, conn *grpc.ClientConn) (any, error) {
	return pb.NewOrcaiPluginClient(conn), nil
}

// LoadedPlugin is a running plugin with its gRPC client.
type LoadedPlugin struct {
	Plugin   discovery.Plugin
	Client   pb.OrcaiPluginClient // nil for CLI wrappers
	Info     *pb.PluginInfo       // nil for CLI wrappers
	gpClient *goplugin.Client
}

// Host manages plugin lifecycle.
type Host struct {
	loaded  []*LoadedPlugin
	busAddr string
}

// New creates a Host that tells plugins to connect to busAddr on start.
func New(busAddr string) *Host {
	return &Host{busAddr: busAddr}
}

// Load starts a plugin and connects to it. CLI wrappers are registered without
// launching a process (the sidebar/tmux layer handles launching them).
//
// Plugin process lifecycle (start/stop) is managed here. Per-step execution
// lifecycle (init/execute/cleanup) is managed by the pipeline runner — do not conflate.
func (h *Host) Load(p discovery.Plugin) error {
	if p.Type == discovery.TypeCLIWrapper {
		h.loaded = append(h.loaded, &LoadedPlugin{Plugin: p})
		return nil
	}

	args := append([]string{}, p.Args...)
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  handshake,
		Plugins:          map[string]goplugin.Plugin{"plugin": &grpcPlugin{}},
		Cmd:              exec.Command(p.Command, args...),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           hclog.NewNullLogger(),
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return err
	}
	raw, err := rpcClient.Dispense("plugin")
	if err != nil {
		client.Kill()
		return err
	}
	pluginClient := raw.(pb.OrcaiPluginClient)

	info, err := pluginClient.GetInfo(context.Background(), &pb.Empty{})
	if err != nil {
		client.Kill()
		return err
	}
	if _, err := pluginClient.Start(context.Background(), &pb.StartRequest{BusAddress: h.busAddr}); err != nil {
		client.Kill()
		return err
	}

	h.loaded = append(h.loaded, &LoadedPlugin{
		Plugin:   p,
		Client:   pluginClient,
		Info:     info,
		gpClient: client,
	})
	return nil
}

// Plugins returns all loaded plugins (native and CLI wrappers).
func (h *Host) Plugins() []*LoadedPlugin {
	return h.loaded
}

// StopAll gracefully stops all running native plugins (Stop RPC then Kill).
func (h *Host) StopAll() {
	for _, p := range h.loaded {
		if p.gpClient != nil {
			if p.Client != nil {
				p.Client.Stop(context.Background(), &pb.Empty{}) //nolint:errcheck
			}
			p.gpClient.Kill()
		}
	}
}
