package tunnel

import (
	"os"
	"sync"
	"time"

	"github.com/micro/cli"
	"github.com/micro/go-micro"
	"github.com/micro/go-micro/client"
	"github.com/micro/go-micro/config/options"
	"github.com/micro/go-micro/proxy/mucp"
	"github.com/micro/go-micro/router"
	"github.com/micro/go-micro/server"
	tun "github.com/micro/go-micro/tunnel"
	"github.com/micro/go-micro/tunnel/transport"
	"github.com/micro/go-micro/util/log"
)

var (
	// Name of the router microservice
	Name = "go.micro.tunnel"
	// Address is the tunnel microservice bind address
	Address = ":8084"
	// Tunnel is the tunnel bind address
	Tunnel = ":9096"
	// Channel is the name of the tunnel session
	Channel = "tunnel"
	// Router is the router gossip bind address
	Router = ":9093"
	// Network is the network id
	Network = "local"
)

// run runs the micro server
func run(ctx *cli.Context, srvOpts ...micro.Option) {
	// Init plugins
	for _, p := range Plugins() {
		p.Init(ctx)
	}

	if len(ctx.GlobalString("server_name")) > 0 {
		Name = ctx.GlobalString("server_name")
	}
	if len(ctx.String("address")) > 0 {
		Address = ctx.String("address")
	}
	if len(ctx.String("network_address")) > 0 {
		Network = ctx.String("network")
	}
	if len(ctx.String("tunnel_address")) > 0 {
		Tunnel = ctx.String("tunnel")
	}
	// default gateway address
	var gateway string
	if len(ctx.String("gateway_address")) > 0 {
		gateway = ctx.String("gateway")
	}

	// create a tunnel
	t := tun.NewTunnel(
		tun.Address(Tunnel),
	)

	// connect the tunnel
	if err := t.Connect(); err != nil {
		log.Logf("[tunnel] failed to connect: %s", err)
		os.Exit(1)
	}

	// listen on tunnel
	l, err := t.Listen(Channel)
	if err != nil {
		log.Logf("[tunnel] failed to listen: %s", err)
		os.Exit(1)
	}

	// TODO: go accept tunnel connections

	// Initialise service
	service := micro.NewService(
		micro.Name(Name),
		micro.Address(Address),
		micro.RegisterTTL(time.Duration(ctx.GlobalInt("register_ttl"))*time.Second),
		micro.RegisterInterval(time.Duration(ctx.GlobalInt("register_interval"))*time.Second),
	)

	// local tunnel router
	r := router.NewRouter(
		router.Address(Router),
		router.Network(Network),
		router.Registry(service.Client().Options().Registry),
		router.Gateway(gateway),
	)

	// create tunnel client with tunnel transport
	tr := transport.NewTransport(
		transport.WithTunnel(t),
	)
	c := client.NewClient(
		client.Transport(tr),
	)

	// local proxy
	localProxy := mucp.NewProxy(
		options.WithValue("proxy.router", r),
		options.WithValue("proxy.client", c),
	)

	// local server
	localSrv := server.NewServer(
		server.WithRouter(localProxy),
	)

	// init server
	service.Init(
		micro.Server(localSrv),
	)

	var wg sync.WaitGroup

	errChan := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		errChan <- service.Run()
	}()

	// we block here until either service or server fails
	if err := <-errChan; err != nil {
		log.Logf("[tunnel] error running the tunnel: %v", err)
	}

	log.Log("[tunnel] attempting to stop the tunnel")

	// TODO: stop the router etc.

	wg.Wait()

	log.Logf("[tunnel] successfully stopped")

}

func Commands(options ...micro.Option) []cli.Command {
	command := cli.Command{
		Name:  "tunnel",
		Usage: "Run the micro network tunnel",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:   "tunnel_address",
				Usage:  "Set the micro tunnel address :9096",
				EnvVar: "MICRO_TUNNEL_ADDRESS",
			},
			cli.StringFlag{
				Name:   "network_address",
				Usage:  "Set the micro network address: local",
				EnvVar: "MICRO_NETWORK_ADDRESS",
			},
			cli.StringFlag{
				Name:   "gateway_address",
				Usage:  "Set the micro default gateway address :9094",
				EnvVar: "MICRO_GATEWAY_ADDRESS",
			},
		},
		Action: func(ctx *cli.Context) {
			run(ctx, options...)
		},
	}

	for _, p := range Plugins() {
		if cmds := p.Commands(); len(cmds) > 0 {
			command.Subcommands = append(command.Subcommands, cmds...)
		}

		if flags := p.Flags(); len(flags) > 0 {
			command.Flags = append(command.Flags, flags...)
		}
	}

	return []cli.Command{command}
}
