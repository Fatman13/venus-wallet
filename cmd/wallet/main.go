package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
	"go.opencensus.io/trace"

	"github.com/filecoin-project/go-address"
	localCli "github.com/filecoin-project/venus-wallet/cli"
	main2 "github.com/filecoin-project/venus-wallet/cmd"
	loclog "github.com/filecoin-project/venus-wallet/log"
	"github.com/filecoin-project/venus-wallet/middleware"
	"github.com/filecoin-project/venus-wallet/version"
)

var errConnectRefused = errors.New("connection refused")

func main() {
	loclog.SetupLogLevels()
	local := []*cli.Command{
		main2.RunCmd,
	}
	jaeger := middleware.SetupJaegerTracing("venus-wallet")
	defer func() {
		if jaeger != nil {
			jaeger.Flush()
		}
	}()
	for _, cmd := range local {
		cmd := cmd
		originBefore := cmd.Before
		cmd.Before = func(cctx *cli.Context) error {
			trace.UnregisterExporter(jaeger)
			jaeger = middleware.SetupJaegerTracing("venus/" + cmd.Name)

			if originBefore != nil {
				return originBefore(cctx)
			}
			return nil
		}
	}
	ctx, span := trace.StartSpan(context.Background(), "/cli")
	defer span.End()

	app := &cli.App{
		Name:    "venus remote-wallet",
		Usage:   "./venus-wallet ",
		Version: version.UserVersion,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"VENUS_WALLET_PATH"},
				Hidden:  true,
				Value:   "~/.venus_wallet",
			},
			&cli.StringFlag{Name: "nettype",
				EnvVars: []string{"VENUS_ADDRESS_TYPE"},
				Value:   "calibnet",
				Usage:   "should be mainnet/calibnet",
			},
		},
		Before: func(c *cli.Context) error {
			address.CurrentNetwork = address.Mainnet
			if c.String("nettype") == "mainnet" {
				address.CurrentNetwork = address.Mainnet
			} else if c.String("nettype") == "calibnet" {
				address.CurrentNetwork = address.Testnet
			} else {
				return fmt.Errorf("nettype should be 'mainnet' or 'calibnet'")
			}
			return nil
		},

		Commands: append(local, localCli.Commands...),
	}
	app.Setup()
	app.Metadata["traceContext"] = ctx

	if err := app.Run(os.Args); err != nil {
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeFailedPrecondition,
			Message: err.Error(),
		})
		if strings.Contains(err.Error(), errConnectRefused.Error()) {
			fmt.Printf("%v. %s\n", err, "Is the venus-wallet running?")
		} else {
			fmt.Println(err)
		}
		os.Exit(1)
	}
}
