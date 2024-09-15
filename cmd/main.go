//go:build linux

package main

import (
	"log"
	"net"
	"os"
	"path"

	"github.com/urfave/cli/v2"

	snapshotsapi "github.com/containerd/containerd/api/services/snapshots/v1"
	"github.com/containerd/containerd/contrib/snapshotservice"
	"github.com/containerd/containerd/snapshots/overlay"
	"github.com/qwqVictor/overlay-read-write-splitting-snapshotter/snapshotter"
	"google.golang.org/grpc"
)

func main() {

	app := &cli.App{
		Name:  "overlay-read-write-splitting-snapshotter",
		Usage: "Run a read write splitting overlay containerd snapshotter",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "root-dir",
				Value: snapshotter.DefaultRootDir,
				Usage: "Adds as an optional label \"containerd.io/snapshot/overlay.upperdir\"",
			},
			&cli.BoolFlag{
				Name:  "async-remove",
				Value: true,
				Usage: "Defers removal of filesystem content until the Cleanup method is called",
			},
			&cli.BoolFlag{
				Name:  "upperdir-label",
				Value: false,
				Usage: "AsynchronousRemove defers removal of filesystem content until the Cleanup method is called",
			},
		},
		Action: func(ctx *cli.Context) error {
			// 创建 snapshotter
			root := ctx.String("root-dir")
			sOpts := []overlay.Opt{}
			if ctx.Bool("async-remove") {
				sOpts = append(sOpts, overlay.AsynchronousRemove)
			}
			if ctx.Bool("upperdir-label") {
				sOpts = append(sOpts, overlay.WithUpperdirLabel)
			}
			sn, err := snapshotter.NewSnapshotter(root, sOpts...)
			if err != nil {
				return err
			}
			// 封装成 grpc service
			service := snapshotservice.FromSnapshotter(sn)
			// 创建一个 rpc server
			rpc := grpc.NewServer()
			// 将 grpc service 注册到 grpc server
			snapshotsapi.RegisterSnapshotsServer(rpc, service)
			// Listen and serve
			socksPath := path.Join(root, snapshotter.SocksFileName)
			err = os.RemoveAll(socksPath)
			if err != nil {
				return err
			}
			l, err := net.Listen("unix", socksPath)
			if err != nil {
				return nil
			}
			return rpc.Serve(l)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
