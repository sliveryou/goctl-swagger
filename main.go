package main

import (
	"fmt"
	"os"
	"runtime"

	cli "github.com/urfave/cli/v2"
	"github.com/zeromicro/goctl-swagger/action"
	"github.com/zeromicro/goctl-swagger/generate"
)

var (
	version  = "20220621"
	commands = []*cli.Command{
		{
			Name:   "swagger",
			Usage:  "generates swagger.json",
			Action: action.Generator,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "host",
					Usage: "api request address",
				},
				&cli.StringFlag{
					Name:  "basepath",
					Usage: "url request prefix",
				},
				&cli.StringFlag{
					Name:  "filename",
					Usage: "swagger save file name",
				},
				&cli.StringFlag{
					Name:  "schemes",
					Usage: "swagger support schemes: http, https, ws, wss",
				},
				&cli.StringFlag{
					Name: "pack", // 开启外层响应包装并指定外层响应结构名称
					Usage: "use outer packaging response and specify the name, " +
						"example: Response",
				},
				&cli.StringFlag{
					Name: "response", // 指定外层响应结构
					Usage: "outer packaging response structure, " +
						"example: " + fmt.Sprintf("%q", generate.DefaultResponseJson),
				},
			},
		},
	}
)

func main() {
	app := cli.NewApp()
	app.Usage = "a plugin of goctl to generate swagger.json"
	app.Version = fmt.Sprintf("%s %s/%s", version, runtime.GOOS, runtime.GOARCH)
	app.Commands = commands
	if err := app.Run(os.Args); err != nil {
		fmt.Printf("goctl-swagger: %+v\n", err)
	}
}
